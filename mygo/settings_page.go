//go:build native

package main

import (
	"fmt"
	"homecast/service/fav"

	"github.com/egoist/mygo/ui"
)

func (a *app) settingsPage(c *ui.Context) {
	ui.Column(c).Fill().Padding(a.resp(16, 10), a.resp(24, 12), 0, a.resp(24, 12)).Gap(10 * uiScale).Children(func() {
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "设置").FontSize(20 * uiScale).Bold().TextColor(clrText)
			ui.Spacer(c).Grow(1)
			// 子 tab：系统设置 / 插件（同一导航项分页显示）
			a.setRowSeg(c, []string{"系统设置", "插件"}, a.settingsTab, func(i int) {
				a.settingsTab = i
				a.win.Invalidate()
			})
		})
		if a.settingsTab == 1 {
			a.pluginPage(c)
		} else {
			a.systemSettings(c)
		}
	})
}

// systemSettings 系统设置内容（原设置页，主题/音质/缓存等）。
func (a *app) systemSettings(c *ui.Context) {
	ui.Scroll(c).Grow(1).Gap(10 * uiScale).Scrollbars(false).Children(func() {
		// 限宽居中（对齐主线 max-w-2xl mx-auto）：宽屏不撑满，条项整体居中
		ui.Column(c).Gap(10 * uiScale).AlignSelf(ui.Center).WidthPercent(100).MaxWidth(672 * uiScale).Children(func() {
			// 1 主题模式（主线 THEMES 三态：自动/明亮/暗黑；持久化 settings kv）
			a.setRow(c, "主题模式")
			a.setRowSeg(c, []string{"自动", "明亮", "暗黑"}, a.themeMode, func(i int) {
				a.themeMode = i
				a.savePref("mygo_theme", i)
				a.win.Invalidate()
			})
			// 4 播放模式（主线 MODES 四态 + 当前模式名）
			a.setRow(c, "播放模式")
			a.setRowText(c, "当前："+a.loopLabel()+"（点按钮切换）")
			a.setRowSeg(c, []string{"顺序播放", "列表循环", "单曲循环", "随机播放"}, a.loopMode, func(i int) {
				a.loopMode = i
				a.pb.SetMode(intToMode(i))
			})
			// 9 歌曲缓存（主线 cache.js：已用/清空未收藏/清空缓存/上限）
			a.setRow(c, "歌曲缓存")
			a.setRowText(c, "自动缓存播放超过 60 秒的歌，再次播放秒开；超上限自动淘汰最旧")
			a.setRowText(c, "已用："+a.cacheUsedLabel())
			a.setRowBtns(c, []string{"清空未收藏", "清空缓存"}, func(i int) {
				if i == 0 {
					a.clearCache(false)
				} else {
					a.clearCache(true)
				}
			})
			a.setRow(c, "缓存上限")
			a.setRowSeg(c, []string{"关闭", "200MB", "500MB", "1GB", "2GB"}, a.cacheLimit, func(i int) {
				a.cacheLimit = i
				a.savePref("mygo_cachelimit", i)
				if a.svc != nil && a.svc.cache != nil {
					a.svc.cache.SetLimitMB([]int{0, 200, 500, 1000, 2000}[i])
				}
				a.win.Invalidate()
			})
			// 10 歌词缓存（主线 lyc：记忆歌词源，LRU 50 首）
			a.setRow(c, "歌词缓存")
			a.setRowText(c, "字幕条与歌词面板共用：自动记忆你选的歌词源，最多 50 首")
			a.setRowText(c, "已缓存："+a.lyricCacheCount()+" 首 · "+a.lyricCacheSize())
			a.setRowBtns(c, []string{"清空歌词缓存"}, func(i int) {
				if a.svc != nil && a.svc.lyricCache != nil {
					a.svc.lyricCache.ClearAll()
					a.win.Invalidate()
				}
			})
			// 11 数据（主线：搜索历史 + 清空）
			a.setRow(c, "数据")
			a.setRowText(c, "搜索历史（"+a.historyCount()+" 条）")
			a.setRowBtns(c, []string{"清空历史"}, func(i int) {
				a.searchHistory = nil
				a.saveHistory()
				a.win.Invalidate()
			})
			// 12 关于（主线）
			a.setRow(c, "关于")
			a.setRowText(c, "mymusic MyGo 原生版 · 紫粉渐变主题 · SSOT 共享主线 playback 域")
		})
	})
}

// █ 设置页区块行（对齐主线 settings.js 的 card 卡片布局）

// setRow 区块标题行。
func (a *app) setRow(c *ui.Context, label string) {
	ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).PaddingX(4 * uiScale).Children(func() {
		ui.Text(c, label).FontSize(15 * uiScale).Bold().TextColor(clrText)
		ui.Spacer(c).Grow(1)
	})
}

// setRowText 区块内说明文字行（对齐主线 text-xs text-gray-500）。
func (a *app) setRowText(c *ui.Context, txt string) {
	ui.Text(c, txt).FontSize(12 * uiScale).TextColor(clrMuted).PaddingX(4 * uiScale)
}

// setRowSeg 分段按钮行（对齐主线 flex gap-2 圆角按钮组）。
func (a *app) setRowSeg(c *ui.Context, opts []string, cur int, cb func(int)) {
	ui.Row(c).Gap(a.resp(10, 8)).PaddingX(4 * uiScale).Wrap().Children(func() {
		for i, opt := range opts {
			i, opt := i, opt
			if segBtn(c, opt, i == cur).Clicked() {
				cb(i)
			}
		}
	})
}

// setRowSwitch 开关行（对齐主线 w-11 h-6 滑动开关）。
func (a *app) setRowSwitch(c *ui.Context, label, hint string, on bool, cb func(bool)) {
	ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).PaddingX(4 * uiScale).Children(func() {
		ui.Text(c, label).FontSize(14 * uiScale).Bold().TextColor(clrText)
		ui.Spacer(c).Grow(1)
		if switchToggle(c, on).Clicked() {
			cb(!on)
		}
	})
	ui.Text(c, hint).FontSize(11 * uiScale).TextColor(clrMuted).PaddingX(4 * uiScale)
}

// setRowBtns 操作按钮行（对齐主线 清空xxx 描边小按钮）。
func (a *app) setRowBtns(c *ui.Context, labels []string, cb func(int)) {
	ui.Row(c).Gap(8 * uiScale).PaddingX(4 * uiScale).Children(func() {
		for i, l := range labels {
			i, l := i, l
			if chip(c, l).Clicked() {
				cb(i)
			}
		}
	})
}

// lyricFontSize 歌词字号当前档（对齐主线 LY_SIZES 14/16/18/22/26）。
func (a *app) lyricFontSize() float32 {
	return float32([]int{14, 16, 18, 22, 26}[a.lyricSize])
}

// cacheUsedLabel 缓存占用（SSOT：cache 域包 Size）。
func (a *app) cacheUsedLabel() string {
	if a.svc == nil || a.svc.cache == nil {
		return "0.0 MB"
	}
	mb := a.svc.cache.Size()
	return fmt.Sprintf("%.1f MB", mb)
}

// clearCache 清空缓存（SSOT：cache 域包；true=全部 ClearAll，false=仅未收藏 ClearUnfav）。
func (a *app) clearCache(all bool) {
	if a.svc == nil || a.svc.cache == nil {
		return
	}
	if all {
		a.svc.cache.ClearAll()
	} else {
		a.svc.cache.ClearUnfav(fav.FavTable) // 收藏表名常量（fav 域包；保留收藏的缓存）
	}
	a.win.Invalidate()
}

// lyricCacheSize 歌词缓存占用。
func (a *app) lyricCacheSize() string {
	if a.svc == nil || a.svc.lyricCache == nil {
		return "0 B"
	}
	n := a.svc.lyricCache.SizeBytes()
	if n < 1<<20 {
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
}

// lyricCacheCount 歌词缓存计数（SSOT：lyriccache 域包）。
func (a *app) lyricCacheCount() string {
	if a.svc == nil || a.svc.lyricCache == nil {
		return "0"
	}
	return fmt.Sprintf("%d", a.svc.lyricCache.Count())
}

// historyCount 搜索历史条数。
func (a *app) historyCount() string {
	return fmt.Sprintf("%d", len(a.searchHistory))
}

// settingRow 设置项行（左标签 + 右文案，对齐主线 settings 行布局）。
func (a *app) settingRow(c *ui.Context, label, value, hint string) {
	ui.Box(c).Fill().Height(56 * uiScale).Radius(12 * uiScale).PaddingX(16 * uiScale).Background(clrCard).Children(func() {
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Children(func() {
			ui.Text(c, label).FontSize(a.resp(15, 14)).TextColor(clrText).Bold().Width(a.resp(140, 104))
			ui.Text(c, value).FontSize(13 * uiScale).TextColor(clrMuted)
			ui.Spacer(c).Grow(1)
			if hint != "" {
				ui.Text(c, hint).FontSize(12 * uiScale).TextColor(clrMuted)
			}
		})
	})
}

// settingRowSwitch 开关行（对齐主线网页字幕条开关）。
func (a *app) settingRowSwitch(c *ui.Context, label string, on bool, hint string, cb func(bool)) {
	ui.Box(c).Fill().Height(56 * uiScale).Radius(12 * uiScale).PaddingX(16 * uiScale).Background(clrCard).Children(func() {
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Children(func() {
			ui.Text(c, label).FontSize(a.resp(15, 14)).TextColor(clrText).Bold().Width(a.resp(140, 104))
			ui.Text(c, hint).FontSize(13 * uiScale).TextColor(clrMuted)
			ui.Spacer(c).Grow(1)
			if switchToggle(c, on).Clicked() {
				cb(!on)
			}
		})
	})
}

// settingRowSeg 分段选择行（对齐主线 settings 的音质/播放模式按钮组）。
func (a *app) settingRowSeg(c *ui.Context, label string, opts []string, cur int, cb func(int)) {
	ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(16 * uiScale).PaddingY(10 * uiScale).Background(clrCard).Children(func() {
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Children(func() {
			ui.Text(c, label).FontSize(a.resp(15, 14)).TextColor(clrText).Bold().Width(a.resp(140, 104))
			ui.Row(c).Gap(6 * uiScale).Children(func() {
				for i, opt := range opts {
					i, opt := i, opt
					if segBtn(c, opt, i == cur).Clicked() {
						cb(i)
					}
				}
			})
		})
	})
}

// switchToggle 开关（开=紫高亮）。
func switchToggle(c *ui.Context, on bool) ui.Element {
	box := ui.Box(c).Size(46, 26).Radius(13 * uiScale)
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		if on {
			p.FillGradient(r, grad(), 13)
		} else {
			p.Fill(r, clrField, 13)
		}
	})
	box.Children(func() {
		knob := ui.Box(c).Size(20, 20).Radius(10 * uiScale).Absolute().Top(3)
		if on {
			knob.Left(23)
		} else {
			knob.Left(3)
		}
		knob.Background(ui.RGB(0xff, 0xff, 0xff))
	})
	return box
}

// segBtn 分段按钮（选中=渐变胶囊）。
func segBtn(c *ui.Context, label string, on bool) ui.Element {
	box := ui.Box(c).Height(32 * uiScale).Radius(8 * uiScale).PaddingX(12 * uiScale).AlignItems(ui.Center).Justify(ui.Center)
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		if on {
			p.FillGradient(r, grad(), 8)
		} else if box.Hovered() {
			p.Fill(r, clrField, 16)
		} else {
			p.Fill(r, clrField, 16)
		}
	})
	box.Children(func() {
		col := clrMuted
		if on {
			col = ui.RGB(0xff, 0xff, 0xff)
		}
		ui.Text(c, label).FontSize(13 * uiScale).TextColor(col)
	})
	return box
}

// castPage 投屏/音箱页：DLNA 设备发现 + 投当前歌 + 小米音箱（SSOT：cast/speaker 域包）。
