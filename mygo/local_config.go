//go:build native

package main

// 本地播放插件配置子页：选择输出声卡。
// mygo audio.Player（Linux）走 PulseAudio 默认 sink（pa_simple_new dev=nil），
// 所以"选声卡"= pactl set-default-sink 切换系统默认输出（本地播放跟随默认）。
// 不塞 mygo 源码、不改库——用系统命令（走 pactl）。

import (
	"os/exec"
	"strings"

	"github.com/egoist/mygo/ui"
)

// audioSink 一个输出声卡（PulseAudio/PipeWire sink）。
type audioSink struct {
	name  string // sink 名（pactl 唯一 id，如 alsa_output.pci-...）
	desc  string // 描述（如 "Ryzen HD Audio Controller Speaker"）
}

// audioSinks 枚举本机输出声卡（pactl list sinks，Name/Description 成对）。
// 失败返回空切片（单声卡环境也正常，无列表 = 显示"默认"）。
func audioSinks() []audioSink {
	out, err := exec.Command("pactl", "list", "sinks").Output()
	if err != nil {
		return nil
	}
	var sinks []audioSink
	var name string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Name:") {
			name = strings.TrimSpace(strings.TrimPrefix(line, "Name:"))
		} else if strings.HasPrefix(line, "Description:") {
			desc := strings.TrimSpace(strings.TrimPrefix(line, "Description:"))
			if name != "" {
				sinks = append(sinks, audioSink{name: name, desc: desc})
			}
			name = ""
		}
	}
	return sinks
}

// defaultSink 当前系统默认输出声卡名（空=失败）。
func defaultSink() string {
	out, err := exec.Command("pactl", "get-default-sink").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// setDefaultSink 切换系统默认输出声卡（本地播放跟随默认 sink）。
func setDefaultSink(name string) error {
	return exec.Command("pactl", "set-default-sink", name).Run()
}

// localConfig 本地播放插件配置子页：输出声卡选择。
// 列出本机声卡（pactl），当前默认高亮，点选即切（系统默认，本地播放跟随）。
func (a *app) localConfig(c *ui.Context) {
	ui.Column(c).Fill().Gap(10 * uiScale).Children(func() {
		a.setRow(c, "输出声卡")

		sinks := audioSinks()
		cur := defaultSink()
		if len(sinks) == 0 {
			ui.Text(c, "未检测到可用声卡（pactl list sinks 无输出）").FontSize(12 * uiScale).TextColor(clrMuted).PaddingX(4 * uiScale)
			return
		}
		for _, sk := range sinks {
			sk := sk
			// 声卡行：名称 + 描述 + 当前默认标记（实心圆点=选中，克制不搞色块）
			row := ui.Box(c).Fill().Radius(8 * uiScale).PaddingX(10 * uiScale).PaddingY(10 * uiScale)
			if row.Hovered() {
				row.Background(clrCardHov)
			}
			row.Children(func() {
				ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
					// 选中标记：默认声卡=实心圆，其它=空心圆（形状差异化，不用颜色高亮）
					if sk.name == cur {
						ui.Box(c).Size(10*uiScale, 10*uiScale).Radius(5 * uiScale).Background(clrPink)
					} else {
						// 空心圆（painter Stroke 描边，无 Background）
						circle := ui.Box(c).Size(10*uiScale, 10*uiScale)
						circle.Draw(func(p *ui.Painter, r ui.Rect) {
							p.Stroke(r, clrMuted, 10*uiScale, 1.5*uiScale)
						})
					}
					ui.Column(c).Grow(1).Gap(2 * uiScale).Children(func() {
						ui.Text(c, sk.desc).FontSize(13 * uiScale).Bold().TextColor(clrText).MaxLines(1)
						ui.Text(c, sk.name).FontSize(11 * uiScale).TextColor(clrMuted).MaxLines(1)
					})
					if sk.name == cur {
						ui.Text(c, "当前输出").FontSize(11 * uiScale).TextColor(clrPink)
					}
				})
			})
			// 点选切换默认声卡（系统默认，本地播放跟随）
			if row.Clicked() && sk.name != cur {
				if err := setDefaultSink(sk.name); err != nil {
					a.status = "切换声卡失败: " + err.Error()
				}
				a.win.Invalidate()
			}
		}
	})
}
