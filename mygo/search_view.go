//go:build native

package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
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
			a.searchPage = 0 // 新搜索回第 1 页
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
					a.searchPage = 0 // 新搜索回第 1 页
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
	// 整页滚动条（对齐 recent 页模式）：整页内容包一个 Scroll，不做组件内部滚动条
	ui.Scroll(c).Grow(1).Scrollbars(false).Children(func() {
		ui.Column(c).Fill().PaddingX(a.resp(24, 12)).PaddingY(a.resp(16, 10)).Gap(14 * uiScale).Children(func() {
			a.searchBar(c)
			a.searchChips(c)
			// 主流风格分类（未搜索时展示）：卡片网格，点击 → 复用现有搜索流程
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "按风格找歌").FontSize(a.resp(18, 16)).Bold().TextColor(clrText)
			})
			// 风格网格（不挤一行：桌面 4 列 → 2 行，紧凑 3 列，手机 2 列）
			cols := a.styleGridCols()
			tracks := make([]ui.Track, cols)
			for i := range tracks {
				tracks[i] = ui.Fr(1)
			}
			ui.Grid(c).ColumnTracks(tracks...).Gap(12 * uiScale).Fill().Children(func() {
				for _, g := range styleGenres {
					g := g
					a.styleCard(c, g, a.recentCardSize(), func() {
						a.query = g.Name
						a.searchPage = 0 // 新搜索回第 1 页
						go a.search()
					})
				}
			})
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
