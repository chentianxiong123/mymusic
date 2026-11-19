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
						// 启用开关（插件设置核心：禁用=不生效）
						on := plugin.IsEnabled(in.Type, in.Name)
						if switchToggle(c, on).Clicked() {
							plugin.SetEnabled(in.Type, in.Name, !on)
							a.savePref("plugin_"+in.Type+"_"+in.Name, map[bool]int{true: 1, false: 0}[!on])
							a.win.Invalidate()
						}
					})
					// 类型特定设置（插件自己的设置）
					a.pluginSettings(c, in)
				})
			})
		}
	})
}

// pluginSettings 插件类型特定设置（B站音质 / 网易云偏移）。
func (a *app) pluginSettings(c *ui.Context, in plugin.Info) {
	if in.Type == "source" && in.Name == "bilibili" {
		// B站源：默认音质偏好
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Children(func() {
			ui.Text(c, "默认音质").FontSize(12 * uiScale).TextColor(clrMuted)
			a.setRowSeg(c, []string{"低", "中", "高"}, a.quality, func(i int) {
				a.quality = i
				a.savePref("mygo_quality", i)
				a.win.Invalidate()
			})
		})
	}
	if in.Type == "lyric" && in.Name == "netease" {
		// 网易云词源：歌词偏移
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Children(func() {
			ui.Text(c, "歌词偏移").FontSize(12 * uiScale).TextColor(clrMuted)
			if chip(c, "−0.1s").Clicked() {
				a.lyricOffset -= 0.1
				a.savePrefFloat("lyric_offset", a.lyricOffset)
				a.win.Invalidate()
			}
			ui.Text(c, fmt.Sprintf("%.1fs", a.lyricOffset)).FontSize(12 * uiScale).TextColor(clrText)
			if chip(c, "+0.1s").Clicked() {
				a.lyricOffset += 0.1
				a.savePrefFloat("lyric_offset", a.lyricOffset)
				a.win.Invalidate()
			}
		})
	}
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
