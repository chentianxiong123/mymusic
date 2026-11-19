//go:build native

package main

import (
	"encoding/base64"
	"fmt"
	"mymusic-mygo/plugin"
	"mymusic-mygo/target"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

func (a *app) castPage(c *ui.Context) {
	// 目标页完全由插件系统派生：枚举已注册目标面板（DLNA/蓝牙/音箱/...），
	// 每类目标插件贡献自己的区块（发现/登录/设备列表/投送）。
	ui.Scroll(c).Grow(1).Scrollbars(false).Children(func() {
		ui.Column(c).Fill().PaddingX(a.resp(24, 12)).PaddingY(a.resp(16, 10)).Gap(12 * uiScale).Children(func() {
			// 当前投屏状态（对齐主线 d-stop：投屏中可停止）
			if a.castOn != "" {
				ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(16 * uiScale).PaddingY(12 * uiScale).Background(clrCard).Children(func() {
					ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(12 * uiScale).Children(func() {
						icon(c, icCast, 18, clrPink)
						ui.Text(c, "正在投屏："+a.castOn).FontSize(14 * uiScale).TextColor(clrText).Bold()
						ui.Spacer(c).Grow(1)
						if chip(c, "停止投屏").Clicked() {
							a.stopCast()
						}
					})
				})
			}
			// 插件派生：每个目标面板渲染自己的区块
			for _, p := range plugin.TargetPanels() {
				p.Render(c, a)
			}
		})
	})
}


// startSpeakerLogin 生成音箱登录二维码（SSOT：xiaoai QRLogin）+ 轮询状态。
func (a *app) startSpeakerLogin() {
	if a.svc == nil || a.svc.qr == nil {
		return
	}
	res, err := a.svc.qr.GenerateQRCode()
	if err != nil {
		a.win.Update(func() { a.qrStatus = "二维码生成失败：" + err.Error() })
		return
	}
	a.win.Update(func() {
		qri, _ := res["qr_image"].(string)
		a.qrImage = qri
		if qri != "" {
			// qr_image 是 "data:image/png;base64,..."（xiaoai qrcode.go 编码），
			// DecodeImage 只收原始 bytes，必须先去掉 data URI 前缀 + base64 解码，
			// 否则解码失败 → 二维码不显示。
			raw := []byte(qri)
			if i := strings.Index(qri, ","); i >= 0 {
				if b, derr := base64.StdEncoding.DecodeString(qri[i+1:]); derr == nil {
					raw = b
				}
			}
			if img, derr := ui.DecodeImage(raw); derr == nil {
				a.covers["__qr__"] = img
			} else {
				a.qrStatus = "二维码解码失败：" + derr.Error()
			}
		}
		a.qrStatus = "等待扫码…"
		a.win.Invalidate()
	})
	// 轮询（CheckStatus 长轮询 120s）；成功后刷新设备
	go func() {
		for {
			st, err := a.svc.qr.CheckStatus()
			if err != nil {
				a.win.Update(func() { a.qrStatus = "登录失败：" + err.Error() })
				return
			}
			status, _ := st["status"].(string)
			msg, _ := st["message"].(string)
			a.win.Update(func() { a.qrStatus = msg })
			switch status {
			case "success":
				// 把 QR token 灌进 speaker auth（对齐主线 hx/speaker.go applyQRToken）
				a.svc.applyQRToken()
				a.win.Update(func() { a.qrStatus = "登录成功，正在刷新设备…" })
				a.discoverCast()
				// 已登录 → 刷新音箱设备
				if a.svc.speakerLoggedIn() {
					go a.refreshSpeakers()
				}
				return
			case "failed":
				return
			}
		}
	}()
}

// importTokenFromClipboard 从剪贴板读 token 粘贴登录（对齐主线 speaker_login.html
// 的 sp-token-input 粘贴入口 + POST /api/v1/speaker/token/import）。
// fillTokenFromClipboard 读剪贴板填入 token 输入框（不登录，用户可编辑后再登录）。
func (a *app) fillTokenFromClipboard() {
	if a.svc == nil || a.svc.qrAuth == nil {
		return
	}
	data, err := mygo.Clipboard.Read(transfer.Text)
	if err != nil {
		a.win.Update(func() { a.status = "读取剪贴板失败：" + err.Error() })
		return
	}
	text, _ := data.Read(transfer.Text)
	tok := strings.TrimSpace(string(text))
	a.win.Update(func() {
		a.tokenInput = tok
		if tok == "" {
			a.status = "剪贴板为空，请先复制音箱 token"
		}
		a.win.Invalidate()
	})
}

// loginWithToken 用输入的 token 登录（对齐主线 sp-token-input textarea 值 → import）。
func (a *app) loginWithToken(tok string) {
	if a.svc == nil || a.svc.qrAuth == nil {
		return
	}
	tok = strings.TrimSpace(tok)
	if tok == "" {
		a.win.Update(func() { a.status = "请输入音箱 token" })
		a.win.Invalidate()
		return
	}
	if err := a.svc.importSpeakerToken(tok); err != nil {
		a.win.Update(func() { a.status = err.Error() })
		a.win.Invalidate()
		return
	}
	a.win.Update(func() { a.status = "token 登录成功，正在刷新设备…" })
	go a.refreshSpeakers()
	a.win.Invalidate()
}

// refreshSpeakers 刷新小米音箱设备列表（speaker 域包 ListDevices）。
func (a *app) refreshSpeakers() {
	if a.svc == nil || a.svc.speaker == nil || !a.svc.speakerLoggedIn() {
		return
	}
	list := a.svc.speaker.ListDevices() // SpeakerService.ListDevices 无错误返回（auth 层处理）
	a.win.Update(func() {
		a.speakers = list
		// 一切皆插件：每台小爱音箱注册为 target 插件（投送目标）
		for _, sp := range list {
			plugin.RegisterTarget(target.NewXiaoai(a.svc.speaker, a.pb, sp.DID, sp.Name))
		}
		a.win.Invalidate()
	})
}

// discoverBluetooth 扫描蓝牙设备（bluetoothctl devices），注册 target 插件。
func (a *app) discoverBluetooth() {
	list := target.ScanBluetooth()
	a.win.Update(func() {
		a.btDevices = list
		for _, d := range list {
			plugin.RegisterTarget(target.NewBluetooth(d.MAC, d.Name))
		}
		a.status = fmt.Sprintf("发现 %d 个蓝牙设备", len(list))
		a.win.Invalidate()
	})
}

// discoverCast 发现 DLNA 设备（cast 域包 Discover，后台执行）。
func (a *app) discoverCast() {
	if a.svc == nil || a.svc.cast == nil {
		return
	}
	_ = a.svc.cast.Discover()
	list := a.svc.cast.DeviceList()
	a.win.Update(func() {
		a.castDevices = list
		// 一切皆插件：每台 DLNA 设备注册为 target 插件（投屏目标）
		for _, d := range list {
			plugin.RegisterTarget(target.NewCast(a.svc.cast, d.UDN, d.Name))
		}
		// 小米音箱设备也拉一次（登录后才有）
		if a.svc.speaker != nil {
			a.speakers = a.svc.speaker.ListDevices()
		}
		a.win.Invalidate()
	})
}

// castTo 投当前播放到指定 DLNA 设备（cast 域包 Cast）。
// queryCastStatus 查 DLNA 设备播放状态（对齐主线 hx/cast/status：cast.Status(udn) → state）。
func (a *app) queryCastStatus(udn string) {
	if a.svc == nil || a.svc.cast == nil {
		return
	}
	status, err := a.svc.cast.Status(udn)
	if err != nil || status == nil {
		a.win.Update(func() { a.castStatus[udn] = "状态不可用" })
		return
	}
	state, _ := status["state"].(string)
	if state == "" {
		state = "就绪"
	}
	a.win.Update(func() {
		if a.castStatus == nil {
			a.castStatus = map[string]string{}
		}
		a.castStatus[udn] = state
	})
}

func (a *app) castTo(udn, name string) {
	if a.current == nil {
		return
	}
	// 一切皆插件：投送走 target 插件表（找 cast + udn 匹配）
	var t target.Target
	for _, item := range plugin.Targets() {
		if item.Name() == "cast" && item.DeviceID() == udn {
			t = item
			break
		}
	}
	if t == nil {
		a.status = "投屏设备未注册"
		a.win.Invalidate()
		return
	}
	if err := t.Play(a.current.bvid, target.PlayOpts{Title: a.current.title}); err != nil {
		a.status = "投屏失败：" + err.Error()
	} else {
		a.castOn = name
		a.status = "已投屏到 " + name
	}
	a.win.Invalidate()
}

// stopCast 停止投屏（DLNA Stop 动作）。
func (a *app) stopCast() {
	if a.svc == nil || a.svc.cast == nil {
		return
	}
	for _, d := range a.castDevices {
		a.svc.cast.Control(d.UDN, "stop", "", 0)
	}
	a.castOn = ""
	a.status = "已停止投屏"
	a.win.Invalidate()
}

// playToSpeaker 播放当前歌到小米音箱（走 target 插件表，不再硬编码）。
func (a *app) playToSpeaker(did, name string) {
	if a.current == nil {
		return
	}
	// 找 did 匹配的 speaker 目标插件（一切皆插件：投送走插件表）
	var t target.Target
	for _, item := range plugin.Targets() {
		if item.Name() == "speaker" && item.DeviceID() == did {
			t = item
			break
		}
	}
	if t == nil {
		a.status = "音箱未注册"
		a.win.Invalidate()
		return
	}
	if err := t.Play(a.current.bvid, target.PlayOpts{Title: a.current.title, NextID: a.pb.NextBVID()}); err != nil {
		a.status = "音箱播放失败：" + err.Error()
	} else {
		a.castOn = name
		a.status = "已在音箱播放：" + name
		// 投送成功 → 同步切核心 source=speaker（消除双响窗口）
		a.pb.SwitchSpeaker(did)
	}
	a.win.Invalidate()
}

// stopSpeaker 停止音箱投送（对齐主线 stop_speaker 命令 → playback.StopSpeaker 唯一实现：
// 音箱停 + source 切回 web；castOn 清空回“未投屏”态）。
func (a *app) stopSpeaker(did string) {
	if a.svc == nil || a.svc.speaker == nil {
		return
	}
	a.pb.StopSpeaker(did)
	a.castOn = ""
	a.status = "已停止投屏"
	a.win.Invalidate()
}

// openCtxMenu 打开右键菜单（对齐主线 contextmenu：点某行）
