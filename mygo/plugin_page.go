//go:build native

package main

import (
	"fmt"

	"mymusic-mygo/plugin"

	"github.com/egoist/mygo/ui"
)

// pluginPage 插件页：列出全部注册插件（音源/歌词/目标），每插件有启用开关 + 类型特定设置。
// 系统设置页同一导航项内分页显示（settingsTab==1 时进入）。
func (a *app) pluginPage(c *ui.Context) {
	// 子页：点击某插件「编辑」后进入，显示该插件配置（复页显示配置）
	if a.pluginDetail != "" {
		a.pluginDetailPage(c)
		return
	}
	ui.Scroll(c).Grow(1).Gap(10 * uiScale).Scrollbars(true).Children(func() {
		ui.Column(c).Gap(10 * uiScale).AlignSelf(ui.Center).WidthPercent(100).MaxWidth(672 * uiScale).Children(func() {
			a.pluginGroup(c, "音源", "source")
			a.pluginGroup(c, "歌词", "lyric")
			a.pluginGroup(c, "输出目标", "target")
		})
	})
}

// pluginGroup 一组插件（音源/歌词/目标分类展示）。
func (a *app) pluginGroup(c *ui.Context, title, kind string) {
	ui.Text(c, title).FontSize(14 * uiScale).Bold().TextColor(clrText).PaddingX(4 * uiScale)
	infos := plugin.InfoNames(kind)
	if len(infos) == 0 {
		ui.Text(c, "无").FontSize(12 * uiScale).TextColor(clrMuted).PaddingX(4 * uiScale)
		return
	}
	ui.Column(c).Gap(8 * uiScale).Children(func() {
		for _, in := range infos {
			in := in
			card := ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(14 * uiScale).PaddingY(12 * uiScale).Background(clrCard)
			if card.Hovered() {
				card.Background(clrCardHov)
			}
			card.Children(func() {
				ui.Column(c).Fill().Gap(8 * uiScale).Children(func() {
					// 头部：图标 + 名称/版本/描述 + 启用开关
					ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
						icon(c, pluginIcon(in.Icon), 18, pluginColor(kind))
						ui.Column(c).Grow(1).Gap(2 * uiScale).Children(func() {
							ui.Text(c, in.Name+" · v"+in.Version).FontSize(14 * uiScale).Bold().TextColor(clrText).MaxLines(1)
							ui.Text(c, in.Desc).FontSize(11 * uiScale).TextColor(clrMuted).MaxLines(2)
						})
						// 编辑按钮：进入该插件的配置子页（复页显示配置）
						if chip(c, "编辑").Clicked() {
							a.pluginDetail = in.Type + "/" + in.Name
							a.win.Invalidate()
						}
						// 启用开关（插件设置核心：禁用=不生效）
						on := plugin.IsEnabled(in.Type, in.Name)
						if switchToggle(c, on).Clicked() {
							plugin.SetEnabled(in.Type, in.Name, !on)
							a.savePref("plugin_"+in.Type+"_"+in.Name, map[bool]int{true: 1, false: 0}[!on])
							a.win.Invalidate()
						}
					})
				})
			})
		}
	})
}


// pluginIcon 插件图标标识 → SVG。
func pluginIcon(name string) *ui.SVG {
	switch name {
	case "bilibili":
		return icSet
	case "netease":
		return icMusic
	case "local":
		return icVol
	case "speaker":
		return icMsg
	case "cast":
		return icCast
	case "bluetooth":
		return icBluetooth
	}
	return icFav
}

// pluginColor 插件类型色。
func pluginColor(kind string) ui.Color {
	switch kind {
	case "source":
		return clrPink
	case "lyric":
		return clrPurple
	}
	return clrBlue
}

// pluginDetailPage 插件配置子页（复页显示配置）：返回按钮 + 插件名 + 完整配置。
func (a *app) pluginDetailPage(c *ui.Context) {
	ui.Column(c).Fill().Gap(10 * uiScale).Children(func() {
		// 顶栏：返回 + 插件名
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
			if chip(c, "‹ 返回").Clicked() {
				a.pluginDetail = ""
				a.win.Invalidate()
			}
			ui.Text(c, a.pluginDetailName()).FontSize(17 * uiScale).Bold().TextColor(clrText)
			ui.Spacer(c).Grow(1)
		})
		// 插件完整配置（按类型/名字分派）
		ui.Scroll(c).Grow(1).Gap(10 * uiScale).Scrollbars(true).Children(func() {
			ui.Column(c).Gap(10 * uiScale).AlignSelf(ui.Center).WidthPercent(100).MaxWidth(672 * uiScale).Children(func() {
				a.pluginConfigForm(c)
			})
		})
	})
}

// pluginDetailName 子页标题：当前编辑插件名。
func (a *app) pluginDetailName() string {
	if a.pluginDetail == "" {
		return ""
	}
	for _, k := range []string{"source", "lyric", "target"} {
		for _, in := range plugin.InfoNames(k) {
			if in.Type+"/"+in.Name == a.pluginDetail {
				return in.Name + " · 配置"
			}
		}
	}
	return "插件配置"
}

// pluginConfigForm 子页内容：当前插件配置表单（网易云歌词完整设置）。
func (a *app) pluginConfigForm(c *ui.Context) {
	switch a.pluginDetail {
	case "source/bilibili":
		a.bilibiliSourceConfig(c)
	case "lyric/netease":
		a.neteaseLyricConfig(c)
	default:
		ui.Text(c, "该插件暂无更多配置").FontSize(12 * uiScale).TextColor(clrMuted).PaddingX(4 * uiScale)
	}
}

// bilibiliSourceConfig B站音源配置：音质归源控制（系统设置已删，由源决定）。
func (a *app) bilibiliSourceConfig(c *ui.Context) {
	// 音质（主线 d-quality / set-q 三档：64k/128k/192k）——源控制，非系统设置
	a.setRow(c, "音质")
	a.setRowText(c, "B站音频码率档（源控制，非系统全局）")
	a.setRowSeg(c, []string{"低", "中", "高"}, a.quality, func(i int) {
		a.quality = i
		a.savePref("mygo_quality", i)
		a.win.Invalidate()
	})
}

// neteaseLyricConfig 网易云歌词完整配置（从系统设置移入：字号/背景/时间显示/偏移）。
func (a *app) neteaseLyricConfig(c *ui.Context) {
	// 歌词偏移
	a.setRow(c, "歌词偏移")
	a.setRowText(c, "歌词与音频不同步时的微调（+ 提前 / − 延后）")
	ui.Row(c).Gap(8 * uiScale).PaddingX(4 * uiScale).Children(func() {
		if chip(c, "−0.1s").Clicked() {
			a.lyricOffset -= 0.1
			a.savePrefFloat("lyric_offset", a.lyricOffset)
			a.win.Invalidate()
		}
		ui.Text(c, fmt.Sprintf("%.1fs", a.lyricOffset)).FontSize(13 * uiScale).TextColor(clrText).PaddingX(8 * uiScale)
		if chip(c, "+0.1s").Clicked() {
			a.lyricOffset += 0.1
			a.savePrefFloat("lyric_offset", a.lyricOffset)
			a.win.Invalidate()
		}
	})
	// 歌词字号（主线 LY_SIZES 14/16/18/22/26）
	a.setRow(c, "歌词字号")
	a.setRowSeg(c, []string{"14px", "16px", "18px", "22px", "26px"}, a.lyricSize, func(i int) {
		a.savePref("mygo_lyricsize", i)
		a.lyricSize = i
		a.win.Invalidate()
	})
	// 歌词背景（主线 set-lybg：封面模糊/纯色）
	a.setRow(c, "歌词背景")
	a.setRowSeg(c, []string{"封面模糊", "纯色"}, a.lyricBg, func(i int) {
		a.lyricBg = i
		a.win.Invalidate()
	})
	// 歌词时间显示（主线 set-lytime 开关）
	a.setRowSwitch(c, "歌词时间显示", "歌词行旁显示时间戳", a.lyricTime, func(on bool) {
		a.lyricTime = on
		a.win.Invalidate()
	})
}
