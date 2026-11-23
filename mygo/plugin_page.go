//go:build native

package main

import (
	"fmt"

	"mymusic-mygo/plugin"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// pluginPage 插件页：3 个分页 tab（音源 / 歌词 / 输出）——每页只显示一类，
// 不再三类全堆一起让用户滚很久。设置页子 tab（settingsTab==1）进入。
func (a *app) pluginPage(c *ui.Context) {
	// 子页：点击某插件「详情」后进入，显示该插件配置（复页显示配置）
	if a.pluginDetail != "" {
		a.pluginDetailPage(c)
		return
	}
	ui.Column(c).Fill().Gap(10 * uiScale).Children(func() {
		// 分页 tab：音源 | 歌词 | 输出（复用设置页的分段选择器）
		a.setRowSeg(c, []string{"音源", "歌词", "输出"}, a.pluginTab, func(i int) {
			a.pluginTab = i
			a.win.Invalidate()
		})
		// 只渲染当前 tab 的那一类插件
		var kind, title string
		switch a.pluginTab {
		case 1:
			kind, title = "lyric", "歌词插件"
		case 2:
			kind, title = "target", "输出目标"
		default:
			kind, title = "source", "音源插件"
		}
		ui.Scroll(c).Grow(1).Scrollbars(false).Children(func() {
			ui.Column(c).AlignSelf(ui.Center).WidthPercent(100).MaxWidth(672 * uiScale).Children(func() {
				a.pluginGroup(c, title, kind)
			})
		})
	})
}

// pluginGroup 一个分区区块：背景 + 色条标题 + 数量 + 官方 ui.List（拖拽排序）。
// 用框架原生 ListState.Reorder：拖拽感应/插入位置提示/边缘滚动/控件点击不冲突全由框架处理，
// 比通用 Draggable 硬凑的偏移换算靠谱得多（用户明确要官方组件）。
func (a *app) pluginGroup(c *ui.Context, title, kind string) {
	infos := plugin.InfoNames(kind)
	section := ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(14 * uiScale).PaddingY(12 * uiScale).Background(clrField)
	section.Children(func() {
		ui.Column(c).Fill().Gap(10 * uiScale).Children(func() {
			// 标题行：色条 + 标题 + 数量 + 拖拽提示
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Children(func() {
				ui.Box(c).Size(3*uiScale, 14*uiScale).Background(pluginColor(kind))
				ui.Text(c, title).FontSize(13 * uiScale).Bold().TextColor(clrText)
				ui.Spacer(c).Grow(1)
				ui.Text(c, "拖动排序").FontSize(10 * uiScale).TextColor(clrMuted)
				ui.Text(c, fmt.Sprintf("%d 个", len(infos))).FontSize(11 * uiScale).TextColor(clrMuted)
			})
			if len(infos) == 0 {
				ui.Text(c, "无").FontSize(12 * uiScale).TextColor(clrMuted).PaddingX(4 * uiScale)
				return
			}
			// 官方 ui.List + ListState.Reorder（每类一个 state，跨帧保持）
			st := a.pluginListState(kind, len(infos))
			ui.List(c, st, len(infos), func(idx int) {
				in := infos[idx]
				row := ui.Box(c).Fill().Radius(8 * uiScale).PaddingX(10 * uiScale).PaddingY(10 * uiScale)
				row.Children(func() {
					ui.Column(c).Fill().Gap(8 * uiScale).Children(func() {
						ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
							// 顺序号（拖拽后自动更新）
							ui.Text(c, fmt.Sprintf("%d", idx+1)).FontSize(11 * uiScale).TextColor(clrMuted).Width(16)
							// 拖拽手柄（视觉提示；ui.List 整行可拖，手柄只是图标）
							ui.Text(c, "⠿").FontSize(14 * uiScale).TextColor(clrMuted)
							icon(c, pluginIcon(in.Icon), 18, pluginColor(kind))
							ui.Column(c).Grow(1).Gap(2 * uiScale).Children(func() {
								ui.Text(c, sourceShortName(in.Name)+" · v"+in.Version).FontSize(14 * uiScale).Bold().TextColor(clrText).MaxLines(1)
								ui.Text(c, in.Desc).FontSize(11 * uiScale).TextColor(clrMuted).MaxLines(2)
							})
							if chip(c, "详情").Clicked() {
								a.pluginDetail = in.Type + "/" + in.Name
								a.win.Invalidate()
							}
							on := plugin.IsEnabled(in.Type, in.Name)
							if switchToggle(c, on).Clicked() {
								plugin.SetEnabled(in.Type, in.Name, !on)
								a.savePref("plugin_"+in.Type+"_"+in.Name, map[bool]int{true: 1, false: 0}[!on])
								a.win.Invalidate()
							}
						})
					})
				})
			}).Height(float32(len(infos)) * 60 * uiScale) // List 需要高度（虚拟列表按行算）
		})
	})
}

// pluginListState 取/建某类型的 ListState（跨帧保持，Key 跟插件走，Reorder 回调重排）。
func (a *app) pluginListState(kind string, n int) *ui.ListState {
	if a.pluginLists == nil {
		a.pluginLists = map[string]*ui.ListState{}
	}
	if st := a.pluginLists[kind]; st != nil {
		return st
	}
	st := &ui.ListState{
		Key: func(row int) any {
			cur := plugin.InfoNames(kind) // 每次读当前顺序（重排后更新）
			if row >= 0 && row < len(cur) {
				return cur[row].Type + "/" + cur[row].Name
			}
			return row
		},
		Reorder: func(rows []int, to int) { a.reorderPlugins(kind, rows, to) },
	}
	a.pluginLists[kind] = st
	return st
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
		ui.Scroll(c).Grow(1).Gap(10 * uiScale).Scrollbars(false).Children(func() {
			ui.Column(c).Gap(10 * uiScale).AlignSelf(ui.Center).WidthPercent(100).MaxWidth(672 * uiScale).Children(func() {
				a.pluginConfigForm(c)
			})
		})
	})
}

// pluginDetailName 子页标题：当前插件名。
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

// pluginConfigForm 子页内容：当前插件配置表单（网易云歌词完整设置 / JS 插件通用表单）。
func (a *app) pluginConfigForm(c *ui.Context) {
	switch a.pluginDetail {
	case "source/bilibili":
		a.bilibiliSourceConfig(c)
	case "lyric/netease":
		a.neteaseLyricConfig(c)
	case "target/speaker":
		a.speakerConfig(c)
	case "target/local":
		a.localConfig(c)
	default:
		// JS 插件（meta.settings 声明）：渲染通用设置表单（系统统一管理）。
		// 解析 pluginDetail = type/name，查设置声明，有则渲染。
		tp, name, ok := splitPluginDetail(a.pluginDetail)
		if ok {
			items := plugin.SettingsOf(tp, name)
			if len(items) > 0 {
				a.jsPluginConfig(c, tp, name, items)
				return
			}
		}
		ui.Text(c, "该插件暂无更多配置").FontSize(12 * uiScale).TextColor(clrMuted).PaddingX(4 * uiScale)
	}
}

// splitPluginDetail 解析 "type/name"（如 "source/demo"）。
func splitPluginDetail(s string) (string, string, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			return s[:i], s[i+1:], true
		}
	}
	return "", "", false
}

// jsPluginConfig JS 插件通用设置表单：遍历声明项渲染（select/bool/segment/text）。
// 系统统一管理：读 GetSetting（默认值），写 SetSetting 落库。
func (a *app) jsPluginConfig(c *ui.Context, tp, name string, items []plugin.Setting) {
	for _, it := range items {
		it := it
		switch it.Type {
		case plugin.SettingBool:
			on := plugin.GetSettingBool(tp, name, it.Key, it.Default == "true")
			a.setRowSwitch(c, it.Name, it.Desc, on, func(b bool) {
				_ = plugin.SetSettingBool(tp, name, it.Key, b)
				a.win.Invalidate()
			})
		case plugin.SettingSelect, plugin.SettingSegment:
			cur := plugin.GetSetting(tp, name, it.Key, it.Default)
			idx := indexOf(it.Options, cur)
			if idx < 0 {
				idx = 0
			}
			a.setRow(c, it.Name)
			if it.Desc != "" {
				a.setRowText(c, it.Desc)
			}
			a.setRowSeg(c, it.Options, idx, func(i int) {
				_ = plugin.SetSetting(tp, name, it.Key, it.Options[i])
				a.win.Invalidate()
			})
		default: // SettingText / 其它：文本输入（简化，读当前值）
			cur := plugin.GetSetting(tp, name, it.Key, it.Default)
			a.setRow(c, it.Name)
			if it.Desc != "" {
				a.setRowText(c, it.Desc)
			}
			ui.TextInput(c, &cur).Grow(1).Height(40 * uiScale).
				FontSize(13 * uiScale).TextColor(clrText).PaddingX(8 * uiScale)
			if chip(c, "保存").Clicked() {
				_ = plugin.SetSetting(tp, name, it.Key, cur)
				a.win.Invalidate()
			}
		}
	}
}

// indexOf 字符串切片查找（返回下标，未找到 -1）。
func indexOf(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return -1
}

// speakerConfig 小爱音箱插件配置子页：登录态（扫码/token/刷新/复制 token）。
// 登录归插件配置（用户要求：小爱音箱登录改到插件页显示，目标页只留设备投送）。
func (a *app) speakerConfig(c *ui.Context) {
	if a.svc == nil {
		return
	}
	if a.svc.speakerLoggedIn() {
		// 已登录：账户头 + 刷新/复制 token
		ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(16 * uiScale).PaddingY(12 * uiScale).Background(clrCard).Children(func() {
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
				icon(c, icMsg, 18, clrPurple)
				ui.Column(c).Grow(1).Gap(2 * uiScale).Children(func() {
					ui.Text(c, "小爱音箱").FontSize(14 * uiScale).Bold().TextColor(clrText)
					ui.Text(c, "已登录 · "+a.svc.speakerUserID()).FontSize(11 * uiScale).TextColor(clrMuted)
				})
				if chip(c, "刷新设备").Clicked() {
					go a.refreshSpeakers()
				}
				if chip(c, "复制 token").Clicked() {
					if tok, err := a.svc.exportSpeakerToken(); err == nil {
						mygo.Clipboard.WriteText(tok)
						a.status = "已复制音箱 token（多端免扫码复用）"
					} else {
						a.status = err.Error()
					}
					a.win.Invalidate()
				}
			})
		})
		// 已登录也显示设备列表（对齐目标页，插件配置页可投送）
		ui.Text(c, "设备").FontSize(14 * uiScale).Bold().TextColor(clrText).PaddingX(4 * uiScale)
		renderSpeakerDevices(c, a)
		return
	}
	// 未登录：QR + token 输入
	if a.qrImage != "" {
		ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(16 * uiScale).PaddingY(12 * uiScale).Background(clrCard).Children(func() {
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(16 * uiScale).Children(func() {
				ui.Box(c).Size(120, 120).Draw(func(p *ui.Painter, r ui.Rect) {
					p.FillRadii(r, [4]float32{8, 8, 8, 8}, clrCard)
					if img, ok := a.covers["__qr__"]; ok {
						p.Clip(r, 8, func() { p.Image(img, r, ui.Cover) })
					}
				})
				ui.Column(c).Grow(1).Gap(6 * uiScale).Children(func() {
					ui.Text(c, "小米音箱扫码登录").FontSize(14 * uiScale).Bold().TextColor(clrText)
					ui.Text(c, "用米家 App 扫描二维码登录").FontSize(12 * uiScale).TextColor(clrMuted)
					ui.Text(c, a.qrStatus).FontSize(12 * uiScale).TextColor(clrPink)
				})
			})
		})
	}
	ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(16 * uiScale).PaddingY(10 * uiScale).Background(clrCard).Children(func() {
		ui.Column(c).Gap(8 * uiScale).Children(func() {
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
				icon(c, icMsg, 18, clrMuted)
				ui.Text(c, "小爱音箱未登录").FontSize(13 * uiScale).TextColor(clrText).Grow(1)
			})
			ui.TextInput(c, &a.tokenInput).Grow(1).Height(60 * uiScale).
				Placeholder("粘贴音箱 token（其他设备导出）…").
				FontSize(12 * uiScale).TextColor(clrText)
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
				ui.Spacer(c).Grow(1)
				if chip(c, "粘贴剪贴板").Clicked() {
					go a.fillTokenFromClipboard()
				}
				if gradButton(c, "token 登录").Clicked() {
					go a.loginWithToken(a.tokenInput)
				}
				if gradButton(c, "扫码登录").Clicked() {
					go a.startSpeakerLogin()
				}
			})
		})
	})
}

// bilibiliSourceConfig B站音源配置：音质归源控制（系统设置已删，由源决定）。
func (a *app) bilibiliSourceConfig(c *ui.Context) {
	// 音质（主线 d-quality / set-q 三档：64k/128k/192k）——源控制，非系统设置
	a.setRow(c, "音质")
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
