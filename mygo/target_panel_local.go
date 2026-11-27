//go:build native

package main

import (
	"mymusic-mygo/plugin"

	"github.com/egoist/mygo/ui"
)

// init 注册本机输出面板（默认播放路径；切回本机 = 停所有远程目标 + 恢复播放）。
func init() {
	plugin.RegisterTargetPanel(plugin.TargetPanel{
		Kind:  "local",
		Order: 10,
		Label: "本机声卡",
		Icon:  "local",
		Render: func(c *ui.Context, h plugin.TargetPanelHost) {
			kind, name := h.ActiveOutput()
			ui.Column(c).AlignSelf(ui.Stretch).Gap(8 * uiScale).Children(func() {
				// 本机是默认输出：任何时刻都显示当前输出是谁（统一状态，归插件自己）
				ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
					icon(c, icLocal, 18, clrPink)
					ui.Column(c).Grow(1).Gap(2 * uiScale).Children(func() {
						ui.Text(c, "本机声卡").FontSize(14 * uiScale).Bold().TextColor(clrText)
						if kind == "local" {
							ui.Text(c, "当前输出").FontSize(11 * uiScale).TextColor(clrGreen)
						} else {
							ui.Text(c, "当前输出："+name).FontSize(11 * uiScale).TextColor(clrMuted).MaxLines(1)
						}
					})
					// 切回本机：任何状态都可用（用户要求保留）
					if chip(c, "切回本机").Clicked() {
						h.SwitchToLocal()
					}
				})
			})
		},
	})
}
