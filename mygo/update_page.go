//go:build native

// update_page 设置 → 更新区块。
//
// 设计原则（用户拍板）：移动端"发现新版本"而非自动更新；提供"禁止检查更新"开关。
//   · 桌面：检查 → 发现 → 安装并重启（mygo.Updater，静默落地）
//   · 移动端：检查 → 发现 → 跳浏览器到 GitHub Release 页手动下载（零权限，OS 升级语义保数据）
//   · 禁止检查更新：开关关闭后整个更新区块收敛为一条提示（不再联网、不再展示输入）
//   · 源与代理全部运行期自填（持久化 settings 表）；校验公钥构建期烙进二进制，用户不可改
//   · 状态/输入放包级 var 而非 app 结构体：避免改 main.go（app 定义在 main.go）
package main

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"homecast/service/conv"
)

// updateUI 更新页状态与用户输入（包级，见文件头说明）。
var updateUI = struct {
	kind     string       // kindGitHub / kindURL
	repo     string       // GitHub owner/name
	url      string       // 自建源根
	proxy    string       // 自定义代理（空=跟随系统）
	disabled bool         // 禁止检查更新
	checking bool         // 检查中
	latest   *mygo.Update // 最近一次 Check 找到的更新（nil=已是最新）
	status   string
}{kind: kindGitHub, status: "尚未检查"}

// updatePrefsLoaded 首次进入更新区块时惰性恢复设置（避免改 main.go 的启动序列）。
var updatePrefsLoaded bool

// isMobile 移动端：更新只能"发现→跳转"，不能"安装"。
func isMobile() bool { return runtime.GOOS == "android" || runtime.GOOS == "ios" }

// savePrefStr 字符串设置落库（savePref 只支持 int，故补一个字符串版）。
func (a *app) savePrefStr(key, v string) {
	if a.db == nil {
		return
	}
	_ = conv.SaveSetting(a.db, key, v)
}

// loadPrefStr 读字符串设置（缺失返回 def）。
func (a *app) loadPrefStr(key, def string) string {
	if a.db == nil {
		return def
	}
	if v, ok := conv.LoadSetting(a.db, key); ok {
		return v
	}
	return def
}

// ensureUpdatePrefs 惰性恢复更新设置（一次）。
func (a *app) ensureUpdatePrefs() {
	if updatePrefsLoaded {
		return
	}
	updatePrefsLoaded = true
	updateUI.repo = a.loadPrefStr("mygo_updrepo", "")
	updateUI.url = a.loadPrefStr("mygo_updurl", "")
	updateUI.proxy = a.loadPrefStr("mygo_updproxy", "")
	updateUI.disabled = a.loadPref("mygo_upddisable", 0) == 1
	if a.loadPref("mygo_updsrc", 0) == 1 {
		updateUI.kind = kindURL
	} else {
		updateUI.kind = kindGitHub
	}
}

// appVersionLabel 当前版本（未打包的开发版显示提示）。
func (a *app) appVersionLabel() string {
	if v := mygo.App.Version(); v != "" {
		return v
	}
	return "开发版（未打包，无内置公钥）"
}

// updateKindIdx 更新源分段当前项。
func (a *app) updateKindIdx() int {
	if updateUI.kind == kindURL {
		return 1
	}
	return 0
}

// saveUpdateInput 把源/代理输入落库并立即生效。
func (a *app) saveUpdateInput() {
	a.savePrefStr("mygo_updrepo", updateUI.repo)
	a.savePrefStr("mygo_updurl", updateUI.url)
	a.savePrefStr("mygo_updproxy", updateUI.proxy)
	a.savePref("mygo_updsrc", a.updateKindIdx())
	a.applySource()
}

// applySource 把用户配置接入 mygo.Updater。
func (a *app) applySource() {
	applyUpdateSource(updateUI.kind, updateUI.repo, updateUI.url, updateUI.proxy)
	var val string
	if updateUI.kind == kindURL {
		val = updateUI.url
	} else {
		val = updateUI.repo
	}
	if val == "" && !updateUI.disabled {
		updateUI.status = "更新源未填（GitHub 填 owner/name；自建填 URL 根）"
	}
}

// releasePageURL GitHub Release 页（移动端"前往下载"用）。
func (a *app) releasePageURL(version string) string {
	if repo := strings.TrimSpace(updateUI.repo); repo != "" {
		return "https://github.com/" + repo + "/releases/tag/mygo-" + version
	}
	if u := strings.TrimRight(updateUI.url, "/"); u != "" {
		return u
	}
	return ""
}

// updateSettings 更新区块（挂在设置 → 关于下）。
func (a *app) updateSettings(c *ui.Context) {
	a.ensureUpdatePrefs()
	a.setRow(c, "更新")
	a.setRowText(c, "当前版本 "+a.appVersionLabel())
	// 禁止检查更新开关（用户拍板：发现而非自动更新，但要能整个关掉）
	a.setRowSwitch(c, "禁止检查更新", "关闭后不再联网检查、不提示新版本", updateUI.disabled, func(on bool) {
		updateUI.disabled = on
		a.savePref("mygo_upddisable", map[bool]int{true: 1, false: 0}[on])
		if on {
			updateUI.latest = nil
			updateUI.status = "已禁止检查更新"
		} else {
			updateUI.status = "尚未检查"
		}
		a.win.Invalidate()
	})
	if updateUI.disabled {
		a.setRowText(c, "更新检查已关闭（可在上方重新打开）")
		return
	}
	// 更新源（用户自填）
	a.setRowSeg(c, []string{"GitHub", "自建 URL"}, a.updateKindIdx(), func(i int) {
		updateUI.kind = []string{kindGitHub, kindURL}[i]
		a.saveUpdateInput()
	})
	ui.Row(c).AlignSelf(ui.Stretch).Gap(8 * uiScale).PaddingX(4 * uiScale).Children(func() {
		if updateUI.kind == kindURL {
			ui.TextInput(c, &updateUI.url).Grow(1).Placeholder("https://your-server.com/mygo").TextColor(clrText)
		} else {
			ui.TextInput(c, &updateUI.repo).Grow(1).Placeholder("owner/name（如 chentianxiong123/mymusic）").TextColor(clrText)
		}
	})
	// 代理（可选；默认跟随系统代理）
	ui.Row(c).AlignSelf(ui.Stretch).Gap(8 * uiScale).PaddingX(4 * uiScale).Children(func() {
		ui.TextInput(c, &updateUI.proxy).Grow(1).Placeholder("代理（空=跟随系统代理）").TextColor(clrText)
	})
	a.setRowBtns(c, a.updateBtnLabels(), func(i int) {
		if updateUI.checking {
			return
		}
		a.saveUpdateInput()
		if updateUI.latest != nil {
			if isMobile() {
				// 移动端：发现 → 跳浏览器手动下载（零权限，OS 升级语义保数据）
				if u := a.releasePageURL(updateUI.latest.Version); u != "" {
					_ = mygo.Shell.OpenExternal(u)
					updateUI.status = "已打开下载页，请下载后覆盖安装（勿卸载）"
				} else {
					updateUI.status = "请先填更新源地址"
				}
				a.win.Invalidate()
			} else {
				go a.installUpdate()
			}
		} else {
			go a.checkUpdate()
		}
	})
	a.setRowText(c, updateUI.status)
}

// updateBtnLabels 更新按钮文案：有更新则提示安装/下载。
func (a *app) updateBtnLabels() []string {
	if updateUI.checking {
		return []string{"检查中…"}
	}
	if updateUI.latest != nil {
		if isMobile() {
			return []string{"前往下载 " + updateUI.latest.Version}
		}
		return []string{"安装 " + updateUI.latest.Version + " 并重启"}
	}
	return []string{"检查新版本"}
}

// checkUpdate 检查更新（后台 goroutine；结果经 win.Update 回主线程）。
func (a *app) checkUpdate() {
	if updateUI.disabled {
		a.win.Update(func() { updateUI.status = "已禁止检查更新" })
		return
	}
	if !mygoUpdaterEnabled() {
		a.win.Update(func() {
			updateUI.status = "此构建未启用更新：需用 mygo build（含 updates.publicKey）产出"
		})
		return
	}
	a.win.Update(func() {
		updateUI.checking = true
		updateUI.status = "正在检查新版本…"
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	upd, err := mygo.Updater.Check(ctx)
	if err != nil {
		a.win.Update(func() {
			updateUI.checking = false
			updateUI.latest = nil
			updateUI.status = "检查失败：" + err.Error()
		})
		return
	}
	a.win.Update(func() {
		updateUI.checking = false
		updateUI.latest = upd
		if upd == nil {
			updateUI.status = "已是最新（" + a.appVersionLabel() + "）"
		} else {
			updateUI.status = "发现新版本 " + upd.Version + "（当前 " + a.appVersionLabel() + "）"
		}
	})
}

// installUpdate 下载并安装，完成后重启（桌面，后台 goroutine）。
func (a *app) installUpdate() {
	upd := updateUI.latest
	if upd == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	lastMB := -1
	err := upd.Install(ctx, func(downloaded, total int64) {
		if total <= 0 {
			return
		}
		mb := downloaded / (1024 * 1024)
		if mb == int64(lastMB) {
			return
		}
		lastMB = int(mb)
		a.win.Update(func() {
			updateUI.status = fmt.Sprintf("下载中 %d/%d MB", mb, total/(1024*1024))
		})
	})
	if err != nil {
		a.win.Update(func() { updateUI.status = "安装失败：" + err.Error() })
		return
	}
	a.win.Update(func() { updateUI.status = "已安装，正在重启…" })
	mygo.App.Relaunch()
}
