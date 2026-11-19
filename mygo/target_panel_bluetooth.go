//go:build native

package main

import (
	"mymusic-mygo/plugin"

	"github.com/egoist/mygo/ui"
)

// init 注册蓝牙目标面板（目标页插件派生）。
func init() {
	plugin.RegisterTargetPanel(plugin.TargetPanel{
		Kind:  "bluetooth",
		Order: 30,
		Label: "蓝牙设备",
		Icon:  "bluetooth",
		Render: func(c *ui.Context, h plugin.TargetPanelHost) {
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Children(func() {
				ui.Text(c, "蓝牙设备").FontSize(14 * uiScale).Bold().TextColor(clrText).PaddingX(4 * uiScale)
				ui.Spacer(c).Grow(1)
				if chip(c, "发现蓝牙").Clicked() {
					h.DiscoverBluetooth()
				}
			})
			if len(h.BtDevices()) == 0 {
				ui.Text(c, "点「发现蓝牙」搜索附近蓝牙设备").FontSize(12 * uiScale).TextColor(clrMuted).PaddingX(4 * uiScale)
			}
			ui.Column(c).Gap(8 * uiScale).Children(func() {
				for _, d := range h.BtDevices() {
					d := d
					card := ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(14 * uiScale).PaddingY(12 * uiScale).Background(clrCard)
					if card.Hovered() {
						card.Background(clrCardHov)
					}
					card.Children(func() {
						ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
							icon(c, icBluetooth, 18, clrBlue)
							ui.Column(c).Grow(1).Gap(2 * uiScale).Children(func() {
								ui.Text(c, d.Name).FontSize(14 * uiScale).Bold().TextColor(clrText).MaxLines(1)
								ui.Text(c, d.MAC).FontSize(11 * uiScale).TextColor(clrMuted).MaxLines(1)
							})
						})
					})
				}
			})
		},
	})
}
