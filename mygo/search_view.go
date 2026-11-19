//go:build native

package main

import (
	"fmt"

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
		sb.Width(672) // 宽窗限宽 672（对齐主线 max-w-2xl）居中
	}
	sb.Children(func() {
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

// searchChips 搜索历史 / 默认热词（对齐主线 search-history）。
func (a *app) searchChips(c *ui.Context) {
	if len(a.searchHistory) > 0 {
		ui.Row(c).AlignSelf(ui.Stretch).Gap(8 * uiScale).Wrap().Justify(ui.Center).Children(func() {
			for _, h := range a.searchHistory {
				h := h
				if chip(c, h).Clicked() {
					a.query = h
					a.resetSearch() // 新搜索回第 1 页（每源独立分页）
					go a.search()
				}
			}
			ui.Column(c).Center().Children(func() {
				if chip(c, "清空").Clicked() {
					a.searchHistory = nil
					a.saveHistory()
					a.win.Invalidate()
				}
			})
		})
	} else {
		// 无历史 → 空（对齐主线 search-history.js：!list.length → box.innerHTML=''，无默认热词）
	}
}

func (a *app) searchEmpty(c *ui.Context) {
	// 整页滚动由外层（search 分支）统一包 Scroll；这里只渲染内容 Column。
	ui.Column(c).Fill().PaddingX(a.resp(24, 12)).PaddingY(a.resp(16, 10)).Gap(14 * uiScale).Children(func() {
		a.searchBar(c)
		a.searchChips(c)
		// 推荐预览（在上）：每插件 1-2 行精简（吸引点击进详情），无推荐插件则隐藏
		a.recommendPreview(c)
		// 风格分类（在下）：卡片网格，点击 → 复用现有搜索流程
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "风格").FontSize(a.resp(18, 16)).Bold().TextColor(clrText)
		})
		// 风格网格（不挤一行：桌面 4 列 → 2 行，紧凑 3 列，手机 2 列）
		// JustifyItems(Center)：固定宽卡片在格内水平居中（用户要求：格子自对齐居中）
		cols := a.styleGridCols()
		tracks := make([]ui.Track, cols)
		for i := range tracks {
			tracks[i] = ui.Fr(1)
		}
		ui.Grid(c).ColumnTracks(tracks...).Gap(12 * uiScale).Fill().JustifyItems(ui.Center).Children(func() {
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
	switch a.tier() {
	case 0:
		return 2
	case 1:
		return 3
	default:
		return 4
	}
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
	// 分区区块容器（背景 clrField，圆角，内边距；对齐 pluginGroup 视觉）
	section := ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(14 * uiScale).PaddingY(12 * uiScale).Background(clrField)
	section.Children(func() {
		ui.Column(c).Fill().Gap(8 * uiScale).Children(func() {
			// 标题行：左侧细色条 + 源名 + 右侧数量（分区标识，非颜色高亮）
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Children(func() {
				ui.Box(c).Size(3*uiScale, 14*uiScale).Background(pluginColor("source")) // 色条（细，克制）
				ui.Text(c, sourceDisplayName(grp.name)).FontSize(13 * uiScale).Bold().TextColor(clrText)
				ui.Spacer(c).Grow(1)
				ui.Text(c, fmt.Sprintf("%d 首", len(grp.songs))).FontSize(11 * uiScale).TextColor(clrMuted)
			})
			// 该源歌曲行（复用 listRow：双击播放/收藏/播放按钮）
			for _, r := range grp.songs {
				a.listRow(c, bilibili.SearchItem{
					BVID: r.ID, Title: r.Title, Author: r.Artist, Pic: r.Cover,
					Duration: r.Duration, Typename: r.Typename, Play: 0,
				})
			}
			// 该源独立分页（用户要求：网易云/bilibili 各自分页显示，页码按钮放该源结果下方）
			if grp.total > srcSearchSize(grp.name) {
				page := a.sourcePage(grp.name)
				pageCount := (grp.total + srcSearchSize(grp.name) - 1) / srcSearchSize(grp.name)
				a.pagerButtons(c, page, pageCount, func(p int) {
					a.searchPages[grp.name] = p
					go a.search()
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
	last := -2
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

// sourceDisplayName 音源显示名：JS 插件显示 meta.name（如"网易云"），否则回退注册 key（bilibili）。
// 注册表 key 保持稳定（netease），仅显示层用中文名；分页/播放仍走 key（grp.name）。
func sourceDisplayName(name string) string {
	if info := plugin.InfoOf("source", name); info.Desc != "" {
		return info.Desc
	}
	return name
}
