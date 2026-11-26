//go:build native

package main

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
	"homecast/bilibili"
	"mymusic-mygo/plugin"
)

func (a *app) searchBar(c *ui.Context) {
	// 响应式（对齐主线 max-w-2xl mx-auto：宽窗限宽 672 居中，窄窗撑满内容区；
	// 搜索框 flex-grow 弹性 + 按钮固定，窗口拉窄时搜索框缩小、按钮不溢出）
	// 胶囊搜索框（保留圆润形态，搜索是通用控件）；内边距 4 → 搜索按钮贴右边缘
	sb := ui.Row(c).Height(52*uiScale).Radius(26*uiScale).Background(clrField).
		Padding(4*uiScale, 4*uiScale, 4*uiScale, 20*uiScale).Gap(12 * uiScale).AlignItems(ui.Center).AlignSelf(ui.Center)
	if a.narrow() {
		sb.WidthPercent(100) // 窄窗撑满内容区（对齐主线窄屏）
	} else {
		sb.Width(672 * uiScale) // 宽窗限宽 672（随文字缩放，对齐主线 max-w-2xl）居中
	}
	sb.Children(func() {
		// 音源下拉框：一次只选一个源，搜索只请求它（不再是全渠道并发拉爆）
		a.sourceSelector(c)
		// 切换源 → 清掉上个源的残留结果（避免显示错误渠道的旧数据）
		if a.lastSearchSource != "" && a.lastSearchSource != a.currentSource() {
			a.results = nil
			a.resultsBySource = nil
			a.searched = false
			a.lastSearchSource = ""
		}
		icon(c, icSearch, 20, clrMuted)
		// 输入框：透明背景 + 去边框（对齐主线 outline-none bg-transparent；
		// 之前 TextInput 默认边框 → 图1 输入框有浅色描边，主线无）
		ui.TextInput(c, &a.query).Grow(1).Background(ui.RGBA(0, 0, 0, 0)).Border(0, ui.Transparent).
			Placeholder("搜索喜欢的音乐…").TextColor(clrText)
		// 搜索按钮：渐变胶囊（跟搜索框配套的主操作）
		if searchBtn(c, "搜索").Clicked() {
			a.resetSearch() // 新搜索回第 1 页（每源独立分页）
			go a.search()
		}
	})
}

// historyContentWidth 估算所有 chip 排一行的总宽（用于让横滑容器贴合内容宽度）。
func (a *app) historyContentWidth() float32 {
	total := float32(0)
	all := append(append([]string{}, a.searchHistory...), "清空")
	for _, h := range all {
		w := float32(0)
		for _, r := range h {
			if r < 128 {
				w += 7
			} else {
				w += 13
			}
		}
		total += w + 24*uiScale + 8*uiScale // chip padding + gap
	}
	return total
}

// searchChips 搜索历史：横滑容器宽度贴合内容（内容少时跟着收缩），外层居中 →
// 内容少时整体居中，内容超出窗口时容器撑满、hover 浮现 < > 箭头横滑。
// （之前 Fill() 撑满 + Justify(Center) 无效：scroll 容器按内容宽布局，Justify 被忽略 → 偏左。）
func (a *app) searchChips(c *ui.Context) {
	if len(a.searchHistory) == 0 {
		return
	}
	renderChips := func() {
		for _, h := range a.searchHistory {
			h := h
			if chip(c, h).Clicked() {
				a.query = h
				a.resetSearch()
				go a.search()
			}
		}
		if chip(c, "清空").Clicked() {
			a.searchHistory = nil
			a.saveHistory()
			a.win.Invalidate()
		}
	}
	step := 240 * uiScale
	rowH := 34 * uiScale
	// 容器宽度 = min(内容宽, 可用宽)（可用宽 = 窗口 - 边距 - 箭头余量）
	contentW := a.historyContentWidth()
	availW := float32(a.winW) - a.resp(48, 24) - 60*uiScale
	if availW < 120 {
		availW = 120
	}
	boxW := contentW
	if boxW > availW {
		boxW = availW
	}
	// 外层居中：内容少时整行居中，内容多时容器撑满可用宽
	ui.Row(c).AlignSelf(ui.Stretch).Justify(ui.Center).Children(func() {
		hoverBox := ui.Box(c).Width(boxW).Height(rowH)
		hoverBox.Children(func() {
			ui.ScrollHorizontal(c).TrackScroll(&a.historyScroll).Fill().Height(rowH).
				Scrollbars(false).Gap(8 * uiScale).Children(renderChips)
			if hoverBox.Hovered() {
				if a.historyScroll.X > 0 {
					lbtn := ui.Box(c).Absolute().Left(-18 * uiScale).Top(-1 * uiScale).Width(28 * uiScale).Height(36 * uiScale).
						Radius(14 * uiScale).Background(ui.RGBA(0, 0, 0, 0.7)).AlignItems(ui.Center).Justify(ui.Center)
					if lbtn.Clicked() {
						a.historyScroll.ScrollTo(a.historyScroll.X-step, 0, ui.Animation{Duration: 180 * time.Millisecond})
					}
					lbtn.Children(func() { icon(c, icChevL, 16, clrText) })
				}
				if a.historyScroll.X < a.historyScroll.MaxX {
					rbtn := ui.Box(c).Absolute().Right(-18 * uiScale).Top(-1 * uiScale).Width(28 * uiScale).Height(36 * uiScale).
						Radius(14 * uiScale).Background(ui.RGBA(0, 0, 0, 0.7)).AlignItems(ui.Center).Justify(ui.Center)
					if rbtn.Clicked() {
						a.historyScroll.ScrollTo(a.historyScroll.X+step, 0, ui.Animation{Duration: 180 * time.Millisecond})
					}
					rbtn.Children(func() { icon(c, icChevR, 16, clrText) })
				}
			}
		})
	})
}

func (a *app) searchEmpty(c *ui.Context) {
	// 整页滚动由外层（search 分支）统一包 Scroll；这里只渲染内容 Column。
	ui.Column(c).Fill().PaddingX(a.resp(24, 12)).PaddingY(a.resp(16, 10)).Gap(14 * uiScale).Children(func() {
		a.searchBar(c)
		a.searchChips(c)
		// 来源页空态三大并列板块：推荐 / 排行 / 分类
		a.recommendSection(c)
		a.rankSection(c)
		// 板块三：分类（风格网格）
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "分类").FontSize(a.resp(18, 16)).Bold().TextColor(clrText)
		})
		// 风格网格（不挤一行：桌面 4 列 → 2 行，紧凑 3 列，手机 2 列）
		// JustifyItems(Center)：固定宽卡片在格内水平居中（用户要求：格子自对齐居中）
		cols := a.styleGridCols()
		tracks := make([]ui.Track, cols)
		for i := range tracks {
			tracks[i] = ui.Fr(1)
		}
		ui.Grid(c).ColumnTracks(tracks...).Gap(8 * uiScale).Fill().JustifyItems(ui.Center).Children(func() {
			for _, g := range styleGenres {
				g := g
				a.styleCard(c, g, a.recentCardSize(), func() {
					a.query = g.Name
					a.resetSearch() // 新搜索回第 1 页（每源独立分页）
					go a.search()
				})
			}
		})
	})
}

// styleGridCols 风格网格列数（不挤一行：桌面 4 列，紧凑 3 列，手机 2 列）。
func (a *app) styleGridCols() int {
	return a.cardCols(110) // 目标卡片 ~110px（用户：继续缩小），列数按宽自适应（偶数）
}

// chip 小圆角标签。
func (a *app) resultsHeader(c *ui.Context) {
	ui.Row(c).AlignSelf(ui.Stretch).PaddingX(a.resp(24, 12)).PaddingY(a.resp(14, 10)).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
		// 返回：回初始搜索页（清空查询 + 结果，后续做推荐系统首页）
		if iconButton2(c, icBack, a.resp(30, 28), clrText).Clicked() {
			a.query = ""
			a.results = nil
			a.searched = false
			a.win.Invalidate()
		}
		ui.Text(c, "搜索结果").FontSize(a.resp(18, 16)).Bold().TextColor(clrText)
		ui.Text(c, fmt.Sprintf("%d 首", len(a.results))).FontSize(13 * uiScale).TextColor(clrMuted)
		ui.Spacer(c).Grow(1)
		if a.status != "" && !a.compact() {
			ui.Text(c, a.status).FontSize(13 * uiScale).TextColor(clrMuted).MaxLines(1).MaxWidth(260)
		}
		if a.compact() {
			ui.Text(c, a.status).FontSize(12 * uiScale).TextColor(clrMuted).MaxLines(1).MaxWidth(120)
		}
		// 播放全部（对齐主线 hidden sm:inline：窄屏只图标）
		if a.compact() {
			if playIconButton(c, icPlayAll, a.resp(34, 30), clrText).Clicked() {
				if len(a.results) > 0 {
					a.playAll(a.results, 0)
				}
			}
		} else if chip(c, "播放全部").Clicked() {
			if len(a.results) > 0 {
				a.playAll(a.results, 0)
			}
		}
	})
}

// playerBar 完整播放条（学主线 htmx 结构）：字幕条 + 上排（音质/循环/静音/音量）+ 进度 + 下排（封面/收藏/字幕/上一首/播放/下一首）。
func (a *app) searchNotFound(c *ui.Context) {
	ui.Column(c).Fill().Center().Gap(10 * uiScale).Children(func() {
		icon(c, icSearch, 44, ui.RGBA(0xff, 0xff, 0xff, 0.08))
		ui.Text(c, "未找到相关音乐").FontSize(17 * uiScale).Bold().TextColor(clrText)
		ui.Text(c, "换个关键词试试").FontSize(13 * uiScale).TextColor(clrMuted)
		ui.Spacer(c).Height(8 * uiScale)
		if gradButton(c, "换个词继续搜").Clicked() {
			a.query = ""
			a.searched = false
			a.results = nil
			a.win.Invalidate()
		}
	})
}

// searchSourceGroup 一个音源的搜索结果分区（插件分组）：色条标题 + 源名 + 数量 + 该源歌曲行 + 该源独立分页。
// 每个源一个独立区块（视觉对齐 pluginGroup 分区：背景 clrField 圆角 + 色条标题 + 数量）。
// 插件（音源）一注册 → plugin.SourceNames() 多一个 → 搜索页自动多一个分区，UI 零改动。
func (a *app) searchSourceGroup(c *ui.Context, grp srcResult) {
	// 呈现层：过滤+排序只作用于本页（数据/分页仍按 API 原样）
	shown := presentSongs(grp.songs)
	// 分区区块容器（背景 clrField，圆角，内边距；对齐 pluginGroup 视觉）
	section := ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(14 * uiScale).PaddingY(12 * uiScale).Background(clrField)
	section.Children(func() {
		ui.Column(c).Fill().Gap(8 * uiScale).Children(func() {
			// 标题行：左侧细色条 + 源名 + 右侧数量（分区标识，非颜色高亮）
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Children(func() {
				ui.Box(c).Size(3*uiScale, 14*uiScale).Background(pluginColor("source")) // 色条（细，克制）
				ui.Text(c, sourceShortName(grp.name)).FontSize(13 * uiScale).Bold().TextColor(clrText)
				ui.Spacer(c).Grow(1)
				if len(shown) < len(grp.songs) {
					ui.Text(c, fmt.Sprintf("显示 %d / 本页 %d 首", len(shown), len(grp.songs))).FontSize(11 * uiScale).TextColor(clrMuted)
				} else {
					ui.Text(c, fmt.Sprintf("%d 首", len(shown))).FontSize(11 * uiScale).TextColor(clrMuted)
				}
			})
			// 该源歌曲行（复用 listRow：双击播放/收藏/播放按钮）
			if len(shown) == 0 && len(grp.songs) > 0 {
				ui.Text(c, "本页内容已过滤").FontSize(12*uiScale).TextColor(clrMuted).AlignSelf(ui.Center)
			}
			for _, r := range shown {
				a.listRow(c, bilibili.SearchItem{
					BVID: r.ID, Title: r.Title, Author: r.Artist, Pic: r.Cover,
					Duration: r.Duration, Typename: r.Typename, Play: 0, Src: grp.name,
				})
			}
			// 该源独立分页：sourcePage 返回 1 基（给 API），pagerButtons 收 0 基（UI）→ 传入减 1、回调加 1。
			if grp.total > srcSearchSize(grp.name) {
				page := a.sourcePage(grp.name) - 1 // UI 0 基
				pageCount := (grp.total + srcSearchSize(grp.name) - 1) / srcSearchSize(grp.name)
				a.pagerButtons(c, page, pageCount, func(p int) {
					a.searchPages[grp.name] = p // searchPages 存 0 基，sourcePage 会 +1 转 API 1 基
					go a.search()
				})
			}
		})
	})
}

// currentSource 返回下拉框选中的源名（默认 bilibili）。
func (a *app) currentSource() string {
	if a.activeSource == "" {
		return "bilibili"
	}
	return a.activeSource
}

// sourceSelector 搜索栏里的音源下拉框——一次只选一个源，搜索只请求它、只显示它。
// 用 SelectBase 自定义外观：trigger 融进胶囊搜索框（无边框/透明背景/中文名+▾），
// 不用框架默认 Select（有边框背景，塞进胶囊里风格冲突）。
func (a *app) sourceSelector(c *ui.Context) {
	names := plugin.SourceNames()
	if len(names) == 0 {
		return
	}
	cur := a.currentSource()
	sel := ui.SelectBase(c, &a.activeSource)
	if a.restoreCfg.LastSource && a.activeSource != "" && a.activeSource != a.lastSavedSource {
		a.savePrefStr("mygo_lastsource", a.activeSource)
		a.lastSavedSource = a.activeSource
	}
	// trigger：撑满搜索框高度（44px）+ 宽点击热区 + hover 反馈——
	// 之前只有文字高度 ~16px，点不准；现在整个源名块都可点。
	sel.Trigger.Height(44*uiScale).Padding(0, 14*uiScale, 0, 8*uiScale).Draw(func(p *ui.Painter, r ui.Rect) {
		if sel.Trigger.Hovered() {
			p.FillRadii(r, [4]float32{8, 8, 8, 8}, clrCardHov)
		}
	}).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(6 * uiScale).Children(func() {
			ui.Text(c, sourceShortName(cur)).FontSize(14 * uiScale).Bold().TextColor(clrText)
			ui.Text(c, "▾").FontSize(12 * uiScale).TextColor(clrMuted)
		})
	})
	// 下拉弹层：深色卡片圆角 + 限高可滚动（插件多时不会超出屏幕看不到）。
	// Scroll.Grow(1) 要求父容器有确定高度；父若只有 MaxHeight（content-sized），
	// 每帧在「内容高度 vs 上限」间重算 → 列表上下跳动。给 panel 确定高度消除。
	popupH := len(names) * 44 + 12 // 每项 44 + 上下 padding 6*2
	if popupH > 412 {
		popupH = 412
	}
	sel.Popup(func(panel ui.Element) {
		panel.Padding(6 * uiScale).Background(clrCard).Radius(12 * uiScale).Height(float32(popupH) * uiScale)
		ui.Scroll(c).Grow(1).Scrollbars(false).Children(func() {
			for _, name := range names {
				name := name
				item := sel.Item(name).Height(44*uiScale).Padding(0, 16*uiScale, 0, 12*uiScale)
				if item.Highlighted() {
					item.Background(clrCardHov)
				}
				item.Children(func() {
					col := clrMuted
					if name == cur {
						col = clrPink // 当前项高亮
					}
					ui.Text(c, sourceShortName(name)).FontSize(14 * uiScale).TextColor(col)
				})
			}
		})
	})
}

// srcSearchSize 某音源每次搜索请求条数（插件自控：读 source/<name>/searchSize 设置，默认 fetchSize）。
// 校验范围（下限 20 保证分页有意义，上限 100 防误配拉爆）。
func srcSearchSize(name string) int {
	n := plugin.GetSettingInt("source", name, "searchSize", fetchSize)
	if n < 20 {
		return fetchSize
	}
	if n > 100 {
		return 100
	}
	return n
}

// sourcePage 某音源当前页（0 基，每源独立分页；默认第 1 页）。
func (a *app) sourcePage(name string) int {
	if p, ok := a.searchPages[name]; ok {
		return p + 1 // 搜索接口 page 从 1 起
	}
	return 1
}

// resetSearch 新搜索重置分页（每源独立分页清空，searchPage 归 0）。
func (a *app) resetSearch() {
	a.searchPage = 0
	a.searchPages = map[string]int{}
}

// pagerButtons 页码按钮式分页（用户要求：给按钮去按，不上下排布、不滚到底部）。
// 当前页实心高亮，其它空心；首尾页号 + 中间省略（>7 页时）；左右箭头。
// 放置：结果区显眼处（header 下方），点击跳页。
func (a *app) pagerButtons(c *ui.Context, page, totalPages int, onGo func(int)) {
	if totalPages <= 1 {
		return
	}
	ui.Row(c).AlignSelf(ui.Center).AlignItems(ui.Center).Gap(6 * uiScale).PaddingY(8 * uiScale).Children(func() {
		// 左箭头（首页/上一页：page=0 时禁用）
		if page > 0 {
			if iconButton2(c, icChevL, 28, clrText).Clicked() {
				onGo(page - 1)
			}
		} else {
			iconButton2(c, icChevL, 28, clrMuted)
		}
		// 页码按钮：当前页高亮，其余可点；>7 页省略中间（首尾 + 当前±1）
		for _, p := range pageList(page, totalPages) {
			if p < 0 {
				ui.Text(c, "…").FontSize(13 * uiScale).TextColor(clrMuted).PaddingX(2 * uiScale)
				continue
			}
			if p == page {
				// 当前页：实心高亮（形状=实心，不用色块）
				cur := ui.Box(c).Width(30 * uiScale).Height(30 * uiScale).Radius(8 * uiScale).Background(clrPink).AlignItems(ui.Center).Justify(ui.Center)
				cur.Children(func() {
					ui.Text(c, fmt.Sprintf("%d", p+1)).FontSize(13 * uiScale).Bold().TextColor(ui.RGB(0xff, 0xff, 0xff))
				})
			} else {
				// 其它页：空心按钮（hover 高亮）
				p := p
				btn := ui.Box(c).Width(30 * uiScale).Height(30 * uiScale).Radius(8 * uiScale).AlignItems(ui.Center).Justify(ui.Center)
				btn.Draw(func(pnt *ui.Painter, r ui.Rect) {
					bg := ui.RGBA(0, 0, 0, 0)
					if btn.Hovered() {
						bg = clrCardHov
					}
					pnt.Fill(r, bg, 8)
				})
				btn.Children(func() {
					ui.Text(c, fmt.Sprintf("%d", p+1)).FontSize(13 * uiScale).TextColor(clrMuted)
				})
				if btn.Clicked() {
					onGo(p)
				}
			}
		}
		// 右箭头（下一页：page=last 时禁用）
		if page < totalPages-1 {
			if iconButton2(c, icChevR, 28, clrText).Clicked() {
				onGo(page + 1)
			}
		} else {
			iconButton2(c, icChevR, 28, clrMuted)
		}
	})
}

// pageList 页码列表：<=7 全显；>7 显首尾 + 当前±1，中间用 -1 表示省略号。
func pageList(page, total int) []int {
	if total <= 7 {
		out := make([]int, total)
		for i := range out {
			out[i] = i
		}
		return out
	}
	// 首尾 + 当前±1，去重排序
	set := map[int]bool{0: true, total - 1: true}
	for i := page - 1; i <= page+1; i++ {
		if i >= 0 && i < total {
			set[i] = true
		}
	}
	out := []int{}
	last := -1 // 从第 0 项开始，前面不应插省略号
	for i := 0; i < total; i++ {
		if set[i] {
			if i-last > 1 {
				out = append(out, -1) // 省略
			}
			out = append(out, i)
			last = i
		}
	}
	return out
}

// sourceShortName 音源短名（下拉框/结果分组标题用）：优先插件 meta.name（中文，
// 如"网易云"），否则内置源固定映射，最后回退 key。Desc 是长描述，不适合标题。
func sourceShortName(name string) string {
	// JS 插件注册时 Info.Name=英文key、Info.Desc=meta.name（中文，如"网易云"）——
	// 取 Desc 才是中文。内置 bilibili 的 Desc 是长描述，特判短名。
	if info := plugin.InfoOf("source", name); info.Desc != "" {
		if name == "bilibili" {
			return "B站"
		}
		return info.Desc
	}
	switch name {
	case "bilibili":
		return "B站"
	case "local":
		return "本地"
	case "netease":
		return "网易云"
	case "kugou":
		return "酷狗"
	case "kuwo":
		return "酷我"
	case "migu":
		return "咪咕"
	case "qianqian":
		return "千千"
	case "qqmusic":
		return "QQ音乐"
	default:
		return name
	}
}
