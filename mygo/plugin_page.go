//go:build native

package main

import (
	"fmt"

	"mymusic-mygo/plugin"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// pluginPage 插件页：一体化列表——一个平台一个条目（能力是标签，不再按类型分页）。
// 设置页子 tab（settingsTab==1）进入。
func (a *app) pluginPage(c *ui.Context) {
	if a.pluginDetail != "" {
		a.pluginDetailPage(c)
		return
	}
	ui.Column(c).Grow(1).Fill().Gap(10 * uiScale).Children(func() {
		a.pluginAllGroup(c)
	})
}

// pluginAllGroup 全部平台一个列表（拖拽排序 + 能力标签 + 启用开关）。
func (a *app) pluginAllGroup(c *ui.Context) {
	infos := plugin.AllInfos()
	section := ui.Box(c).Fill().Grow(1).Radius(12 * uiScale).PaddingX(14 * uiScale).PaddingY(12 * uiScale).Background(clrField)
	section.Children(func() {
		ui.Column(c).Fill().Gap(10 * uiScale).Children(func() {
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Children(func() {
				ui.Box(c).Size(3*uiScale, 14*uiScale).Background(clrPink)
				ui.Text(c, "插件").FontSize(13 * uiScale).Bold().TextColor(clrText)
				ui.Spacer(c).Grow(1)
				ui.Text(c, "拖动排序").FontSize(10 * uiScale).TextColor(clrMuted)
				ui.Text(c, fmt.Sprintf("%d 个", len(infos))).FontSize(11 * uiScale).TextColor(clrMuted)
			})
			if len(infos) == 0 {
				ui.Text(c, "无").FontSize(12 * uiScale).TextColor(clrMuted).PaddingX(4 * uiScale)
				return
			}
			st := a.pluginListState("all", len(infos))
			ui.List(c, st, len(infos), func(idx int) {
				in := infos[idx]
				row := ui.Box(c).Fill().Radius(8 * uiScale).PaddingX(10 * uiScale).PaddingY(10 * uiScale)
				row.Children(func() {
					ui.Column(c).Fill().Gap(8 * uiScale).Children(func() {
						ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
							ui.Text(c, fmt.Sprintf("%d", idx+1)).FontSize(11 * uiScale).TextColor(clrMuted).Width(16)
							ui.Text(c, "⠿").FontSize(14 * uiScale).TextColor(clrMuted)
							icon(c, pluginIcon(in.Icon), 18, clrBlue)
							ui.Column(c).Grow(1).Gap(2 * uiScale).Children(func() {
								name := in.DisplayName
								if name == "" {
									name = in.Name
								}
								ui.Text(c, name).FontSize(14 * uiScale).Bold().TextColor(clrText).MaxLines(1)
								ui.Text(c, capabilityLabels(in.Capabilities)).FontSize(10 * uiScale).TextColor(clrMuted)
								ui.Text(c, in.Desc+" · v"+in.Version).FontSize(11 * uiScale).TextColor(clrMuted).MaxLines(2)
							})
							if chip(c, "详情").Clicked() {
								a.pluginDetail = in.Name // 一体化：直接用平台 key
								a.win.Invalidate()
							}
							on := plugin.IsEnabled("", in.Name)
							if switchToggle(c, on).Clicked() {
								plugin.SetEnabled("", in.Name, !on)
								a.savePref("plugin_"+in.Name, map[bool]int{true: 1, false: 0}[!on])
								a.win.Invalidate()
							}
						})
					})
				})
			}).Grow(1)
		})
	})
}

// capabilityLabels 能力列表 → 中文标签。
func capabilityLabels(caps []string) string {
	labels := map[string]string{"source": "音源", "lyric": "歌词", "recommend": "推荐", "target": "输出"}
	out := ""
	for _, c := range caps {
		if l, ok := labels[c]; ok {
			if out != "" {
				out += " · "
			}
			out += l
		}
	}
	return out
}

// pluginGroup 一个分区区块：背景 + 色条标题 + 数量 + 官方 ui.List（拖拽排序）。
// 用框架原生 ListState.Reorder：拖拽感应/插入位置提示/边缘滚动/控件点击不冲突全由框架处理，
// 比通用 Draggable 硬凑的偏移换算靠谱得多（用户明确要官方组件）。
func (a *app) pluginGroup(c *ui.Context, title, kind string) {
	infos := plugin.InfoNames(kind)
	section := ui.Box(c).Fill().Grow(1).Radius(12 * uiScale).PaddingX(14 * uiScale).PaddingY(12 * uiScale).Background(clrField)
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
								ui.Text(c, sourceShortName(in.Name)).FontSize(14 * uiScale).Bold().TextColor(clrText).MaxLines(1)
								ui.Text(c, in.Desc+" · v"+in.Version).FontSize(11 * uiScale).TextColor(clrMuted).MaxLines(2)
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
			}).Grow(1) // List 自带滚动 + 自动测量行高；不硬设 Height（猜大留白/猜小裁行）
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
			var cur []plugin.Info
			if kind == "all" {
				cur = plugin.AllInfos() // 一体化：全部平台一个列表
			} else {
				cur = plugin.InfoNames(kind)
			}
			if row >= 0 && row < len(cur) {
				return cur[row].Name // 平台 key
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

// pluginDetailName 子页标题：平台名 · 配置。
func (a *app) pluginDetailName() string {
	if a.pluginDetail == "" {
		return ""
	}
	in := plugin.InfoOf("", a.pluginDetail)
	name := in.DisplayName
	if name == "" {
		name = a.pluginDetail
	}
	return name + " · 配置"
}

// pluginConfigForm 子页内容：一个平台的所有能力配置聚在一起（音源/歌词/登录/绑定/输出）。
func (a *app) pluginConfigForm(c *ui.Context) {
	name := a.pluginDetail // 一体化：平台 key
	if name == "" {
		return
	}
	// 兼容旧 pluginDetail = type/name 形式（书签/旧状态）
	if tp, n, ok := splitPluginDetail(name); ok {
		name = n
		_ = tp
	}
	// 登录 + 歌词绑定（音源插件）
	if plugin.HasCapability(name, "source") || plugin.InfoOf("", name).Type == "source" {
		a.qrLoginSection(c, name)
		a.lyricBindSection(c, name)
	}
	// 平台专属配置（按平台 key 分派，不再按 type/name）
	switch name {
	case "bilibili":
		a.bilibiliSourceConfig(c)
	case "netease":
		a.neteaseSourceConfig(c)
		a.neteaseLyricConfig(c) // 网易云既是音源又是歌词源，两个配置都显示
	case "speaker":
		a.speakerConfig(c)
	case "local":
		a.localConfig(c)
	default:
		// JS 插件（meta.settings 声明）：按平台 key 找设置声明
		found := false
		for _, tp := range []string{"source", "lyric", "target", "recommend"} {
			if items := plugin.SettingsOf(tp, name); len(items) > 0 {
				a.jsPluginConfig(c, tp, name, items)
				found = true
				break
			}
		}
		if !found {
			ui.Text(c, "该插件暂无更多配置").FontSize(12 * uiScale).TextColor(clrMuted).PaddingX(4 * uiScale)
		}
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
				ui.Box(c).Size(120*uiScale, 120*uiScale).Draw(func(p *ui.Painter, r ui.Rect) {
					p.FillRadii(r, [4]float32{8, 8, 8, 8}, clrCard)
					if img, ok := a.covers["__qr__"]; ok {
						p.Clip(r, 8, func() { p.Image(img, r, ui.Cover) })
					}
				})
				ui.Column(c).Grow(1).Gap(6 * uiScale).Children(func() {
					ui.Text(c, "小米音箱扫码登录").FontSize(14 * uiScale).Bold().TextColor(clrText)
					ui.Text(c, "用米家 App 扫描二维码登录").FontSize(12 * uiScale).TextColor(clrMuted)
					ui.Text(c, a.qrStatus).FontSize(12 * uiScale).TextColor(clrPink)
					if chip(c, "复制二维码").Clicked() {
						a.copySpeakerQRImage()
					}
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
	// 登录 Cookie（有=登录态/高清，空=匿名）
	a.setRow(c, "登录 Cookie")
	a.setRowText(c, "粘贴 B站登录态 Cookie（登录 www.bilibili.com 后从浏览器复制），空=匿名")
	cookie := plugin.GetCookie("bilibili")
	ui.TextInput(c, &cookie).Grow(1).Height(38 * uiScale).FontSize(12 * uiScale).TextColor(clrText).Placeholder("SESSDATA=...; buvid3=...")
	if chip(c, "保存").Clicked() {
		a.applySourceCookie("bilibili", cookie)
		a.win.Invalidate()
	}
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

// neteaseSourceConfig 网易云 Go 音源配置（音质/搜索条数/Cookie；扫码登录在上方登录区块）。
func (a *app) neteaseSourceConfig(c *ui.Context) {
	a.setRow(c, "音质")
	q := plugin.GetSetting("source", "netease", "quality", "320k")
	qi := indexOf([]string{"128k", "320k"}, q)
	if qi < 0 {
		qi = 1
	}
	a.setRowSeg(c, []string{"128k", "320k"}, qi, func(i int) {
		_ = plugin.SetSetting("source", "netease", "quality", []string{"128k", "320k"}[i])
		a.win.Invalidate()
	})
	a.setRow(c, "搜索条数")
	s := plugin.GetSetting("source", "netease", "searchSize", "40")
	si := indexOf([]string{"20", "40", "60"}, s)
	if si < 0 {
		si = 1
	}
	a.setRowSeg(c, []string{"20", "40", "60"}, si, func(i int) {
		_ = plugin.SetSetting("source", "netease", "searchSize", []string{"20", "40", "60"}[i])
		a.win.Invalidate()
	})
}

// copySpeakerQRImage 复制音箱扫码登录的二维码（共用统一 PNG 复制实现）。
func (a *app) copySpeakerQRImage() {
	a.copyPNGToClipboard(a.qrSpeakerPNG, "已复制音箱二维码（可粘贴到聊天窗口用手机扫）")
}
