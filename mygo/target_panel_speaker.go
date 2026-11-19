//go:build native

package main

import (
	"mymusic-mygo/plugin"

	"github.com/egoist/mygo/ui"
)

// init 注册小爱音箱目标面板（目标页插件派生）。
func init() {
	plugin.RegisterTargetPanel(plugin.TargetPanel{
		Kind:  "speaker",
		Order: 10,
		Label: "小爱音箱",
		Icon:  "speaker",
		Render: func(c *ui.Context, h plugin.TargetPanelHost) {
			// 区块标题「小爱音箱」+ 刷新（对齐 DLNA/蓝牙区块标题样式）
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Children(func() {
				ui.Text(c, "小爱音箱").FontSize(14 * uiScale).Bold().TextColor(clrText).PaddingX(4 * uiScale)
				ui.Spacer(c).Grow(1)
				if chip(c, "刷新设备").Clicked() {
					h.RefreshSpeakers()
				}
			})
			// 登录态在插件页的小爱音箱插件项显示；目标页只留设备投送/停止。
			if h.SpeakerLoggedIn() {
				renderSpeakerDevices(c, h)
			} else {
				ui.Text(c, "未登录（去设置 → 插件 → 小爱音箱登录）").FontSize(12 * uiScale).TextColor(clrMuted).PaddingX(4 * uiScale)
			}
		},
	})
}

// renderSpeakerDevices 已登录设备网格（对齐主线 speaker_devices.html）。
func renderSpeakerDevices(c *ui.Context, h plugin.TargetPanelHost) {
	if len(h.Speakers()) == 0 {
		ui.Text(c, "音箱：点「刷新设备」获取设备列表").FontSize(12 * uiScale).TextColor(clrMuted).PaddingX(4 * uiScale)
	}
	ui.Grid(c).ColumnTracks(ui.Fr(1), ui.Fr(1)).Gap(10 * uiScale).Fill().Children(func() {
		for _, sp := range h.Speakers() {
			sp := sp
			card := ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(14 * uiScale).PaddingY(12 * uiScale).Background(clrCard)
			card.Children(func() {
				ui.Column(c).Fill().Gap(8 * uiScale).Children(func() {
					ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
						icon(c, icVol, 18, clrMuted)
						ui.Column(c).Grow(1).Gap(2 * uiScale).Children(func() {
							ui.Text(c, sp.Name).FontSize(14 * uiScale).Bold().TextColor(clrText).MaxLines(1)
							ui.Text(c, sp.Hardware).FontSize(11 * uiScale).TextColor(clrMuted).MaxLines(1)
						})
						if sp.IsOnline {
							statusBadge(c, "在线", clrGreen)
						} else {
							statusBadge(c, "离线", clrMuted)
						}
					})
					ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Children(func() {
						if chip(c, "投送").Clicked() {
							h.PlayToSpeaker(sp.DID, sp.Name)
						}
						if chip(c, "停止").Clicked() {
							h.StopSpeaker(sp.DID)
						}
					})
					if h.SpkDID() == sp.DID {
						var label string
						if h.IsPlaying() {
							label = "▶ 播放中"
							if h.NextBVID() != "" {
								label = "▶ 播放中 · 播完自动连播"
							}
						} else {
							label = "⏸ 已暂停"
						}
						ui.Text(c, label).FontSize(11 * uiScale).TextColor(clrPink).Bold()
					}
				})
			})
		}
	})
}
