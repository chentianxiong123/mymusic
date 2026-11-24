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
			ui.Column(c).AlignSelf(ui.Stretch).Gap(8 * uiScale).Children(func() {
				ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
					icon(c, icLocal, 18, clrPink)
					ui.Column(c).Grow(1).Gap(2 * uiScale).Children(func() {
						title := "未播放"
						if h.HasCurrent() {
							title = h.CurrentTitle()
						}
						ui.Text(c, title).FontSize(14 * uiScale).Bold().TextColor(clrText).MaxLines(1)
						state := "已停止"
						col := clrMuted
						if h.IsPlaying() {
							state = "播放中"
							col = clrGreen
						}
						ui.Text(c, state).FontSize(11 * uiScale).TextColor(col)
					})
					if chip(c, "播放/暂停").Clicked() {
						h.ToggleLocal()
					}
					if chip(c, "切回本机").Clicked() {
						h.SwitchToLocal()
					}
				})
			})
		},
	})
}
