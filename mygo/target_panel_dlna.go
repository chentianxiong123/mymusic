//go:build native

package main

import (
	"fmt"

	"mymusic-mygo/plugin"

	"github.com/egoist/mygo/ui"
)

// init 注册 DLNA 目标面板（目标页插件派生）。
func init() {
	plugin.RegisterTargetPanel(plugin.TargetPanel{
		Kind:  "dlna",
		Order: 20,
		Label: "DLNA 设备",
		Icon:  "cast",
		Render: func(c *ui.Context, h plugin.TargetPanelHost) {
			kind, name := h.ActiveOutput()
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Children(func() {
				ui.Text(c, "DLNA 设备").FontSize(14 * uiScale).Bold().TextColor(clrText).PaddingX(4 * uiScale)
				ui.Spacer(c).Grow(1)
				if chip(c, "发现设备").Clicked() {
					h.DiscoverCast()
				}
			})
			// 激活态归插件自己：投屏中显示状态 + 停止（不再写死在页面顶部）
			if kind == "dlna" {
				ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(16 * uiScale).PaddingY(10 * uiScale).Background(clrCard).Children(func() {
					ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
						icon(c, icCast, 18, clrPink)
						ui.Text(c, "正在投屏："+name).FontSize(13 * uiScale).Bold().TextColor(clrText).Grow(1).MaxLines(1)
						if chip(c, "停止").Clicked() {
							h.StopCast()
						}
					})
				})
			}
			if len(h.CastDevices()) == 0 {
				ui.Text(c, "点「发现设备」搜索同网段 DLNA 播放器/电视").FontSize(12 * uiScale).TextColor(clrMuted).PaddingX(4 * uiScale)
			}
			ui.Column(c).Gap(8 * uiScale).Children(func() {
				for _, d := range h.CastDevices() {
					d := d
					card := ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(14 * uiScale).PaddingY(12 * uiScale).Background(clrCard)
					if card.Hovered() {
						card.Background(clrCardHov)
					}
					card.Children(func() {
						ui.Column(c).Fill().Gap(8 * uiScale).Children(func() {
							// 头部：图标 + 名称/IP + 状态徽标（对齐主线 cast-status span）
							ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
								icon(c, icCast, 18, clrMuted)
								ui.Column(c).Grow(1).Gap(2 * uiScale).Children(func() {
									ui.Text(c, d.Name).FontSize(14 * uiScale).Bold().TextColor(clrText).MaxLines(1)
									ui.Text(c, d.IP+":"+fmt.Sprintf("%d", d.Port)).FontSize(11 * uiScale).TextColor(clrMuted).MaxLines(1)
								})
								stateText, ok := h.CastStatusMap()[d.UDN]
								if !ok || stateText == "" {
									stateText = "在线"
								}
								statusBadge(c, stateText, clrGreen)
							})
							// 按钮行：「查状态」+「投送」（对齐主线 hx/cast/status + cast-play-btn）
							ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Children(func() {
								if chip(c, "查状态").Clicked() {
									h.QueryCastStatus(d.UDN)
								}
								if chip(c, "投送").Clicked() {
									h.CastTo(d.UDN, d.Name)
								}
							})
						})
					})
				}
			})
		},
	})
}
