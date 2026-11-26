//go:build native

package main

// 来源页空态三大并列板块：推荐 / 排行 / 分类。
//   - 推荐：个性化推荐插件（插件自绘；当前无插件→空态，不塞榜单/统计冒充）
//   - 排行：公共榜单面板（chartPanels，网易云/酷狗下拉单选）
//   - 分类：风格网格（在 search_view.go）
// 三者职责分离，互不混用。

import (
	"github.com/egoist/mygo/ui"
	"homecast/service/playback"
	"mymusic-mygo/plugin"
)

// currentRecommend 返回下拉选中的推荐插件（默认第一个）。
func (a *app) currentRecommend() plugin.Recommend {
	recs := plugin.Recommends()
	if len(recs) == 0 {
		return nil
	}
	for _, r := range recs {
		if r.Name() == a.activeRecommend {
			return r
		}
	}
	return recs[0]
}

// recommendSelector 推荐插件下拉（一次选一个）。
func (a *app) recommendSelector(c *ui.Context) {
	recs := plugin.Recommends()
	if len(recs) == 0 {
		return
	}
	names := make([]string, 0, len(recs))
	for _, r := range recs {
		names = append(names, r.Name())
	}
	cur := a.currentRecommend().Name()
	sel := ui.SelectBase(c, &a.activeRecommend)
	sel.Trigger.Height(32 * uiScale).PaddingX(10 * uiScale).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(5 * uiScale).Children(func() {
			ui.Text(c, cur).FontSize(12 * uiScale).Bold().TextColor(clrText)
			ui.Text(c, "▾").FontSize(10 * uiScale).TextColor(clrMuted)
		})
	})
	popupH := len(names)*40 + 12
	if popupH > 360 {
		popupH = 360
	}
	sel.Popup(func(panel ui.Element) {
		panel.Padding(5 * uiScale).Background(clrCard).Radius(10 * uiScale).Height(float32(popupH) * uiScale)
		ui.Scroll(c).Grow(1).Scrollbars(false).Children(func() {
			for _, name := range names {
				name := name
				item := sel.Item(name).Height(40 * uiScale).PaddingX(10 * uiScale)
				if item.Highlighted() || name == cur {
					item.Background(clrCardHov)
				}
				item.Children(func() {
					ui.Text(c, name).FontSize(12 * uiScale).TextColor(clrText)
				})
			}
		})
	})
}

// recommendSection 板块一：推荐（插件自绘；无插件显示空态，不塞别的冒充）。
func (a *app) recommendSection(c *ui.Context) {
	ui.Column(c).AlignSelf(ui.Stretch).Gap(8 * uiScale).Children(func() {
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
			ui.Text(c, "推荐").FontSize(a.resp(18, 16)).Bold().TextColor(clrText)
			ui.Spacer(c).Grow(1)
			if len(plugin.Recommends()) > 0 {
				a.recommendSelector(c)
			}
		})
		rec := a.currentRecommend()
		if rec == nil {
			ui.Text(c, "暂无推荐内容").FontSize(12 * uiScale).TextColor(clrMuted)
			return
		}
		section := ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(14 * uiScale).PaddingY(12 * uiScale).Background(clrField)
		section.Children(func() { rec.Render(c, a, true) })
		if section.Clicked() {
			a.recommendDetailOpen = true
			a.win.Invalidate()
		}
	})
}

// rankSection 板块二：排行（公共榜单，平台下拉 + 榜单下拉，一次一个）。
func (a *app) rankSection(c *ui.Context) {
	if len(chartPanels) == 0 {
		return
	}
	names := make([]string, 0, len(chartPanels))
	for _, p := range chartPanels {
		names = append(names, p.Name())
	}
	if a.activeChart == "" || a.activeChart == names[0] {
		// activeChart 空 → 默认第一个；同时同步（避免下拉显示空）
		a.activeChart = names[0]
	}
	var selected *chartPanel
	for _, p := range chartPanels {
		if p.Name() == a.activeChart {
			selected = p
			break
		}
	}
	ui.Column(c).AlignSelf(ui.Stretch).Gap(8 * uiScale).Children(func() {
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
			ui.Text(c, "排行").FontSize(a.resp(18, 16)).Bold().TextColor(clrText)
			ui.Spacer(c).Grow(1)
			sel := ui.SelectBase(c, &a.activeChart)
			sel.Trigger.Height(32 * uiScale).PaddingX(10 * uiScale).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(5 * uiScale).Children(func() {
					ui.Text(c, a.activeChart).FontSize(12 * uiScale).Bold().TextColor(clrText)
					ui.Text(c, "▾").FontSize(10 * uiScale).TextColor(clrMuted)
				})
			})
			popupH := len(names)*40 + 12
			if popupH > 360 {
				popupH = 360
			}
			sel.Popup(func(panel ui.Element) {
				panel.Padding(5 * uiScale).Background(clrCard).Radius(10 * uiScale).Height(float32(popupH) * uiScale)
				ui.Scroll(c).Grow(1).Scrollbars(false).Children(func() {
					for _, name := range names {
						name := name
						item := sel.Item(name).Height(40 * uiScale).PaddingX(10 * uiScale)
						if item.Highlighted() || name == a.activeChart {
							item.Background(clrCardHov)
						}
						item.Children(func() {
							ui.Text(c, name).FontSize(12 * uiScale).TextColor(clrText)
						})
					}
				})
			})
		})
		if selected != nil {
			ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(14 * uiScale).PaddingY(12 * uiScale).Background(clrField).Children(func() {
				selected.Render(c, a, true)
			})
		}
	})
}

// recommendDetail 推荐详情页（只显示选中的推荐插件）。
func (a *app) recommendDetail(c *ui.Context) {
	rec := a.currentRecommend()
	ui.Scroll(c).Grow(1).Scrollbars(false).Children(func() {
		ui.Column(c).Fill().PaddingX(a.resp(24, 12)).PaddingY(a.resp(16, 10)).Gap(14 * uiScale).Children(func() {
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
				if iconButton2(c, icBack, a.resp(30, 28), clrText).Clicked() {
					a.recommendDetailOpen = false
					a.win.Invalidate()
				}
				ui.Text(c, "推荐").FontSize(20 * uiScale).Bold().TextColor(clrText)
				ui.Spacer(c).Grow(1)
				if len(plugin.Recommends()) > 0 {
					a.recommendSelector(c)
				}
			})
			if rec != nil {
				ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(14 * uiScale).PaddingY(12 * uiScale).Background(clrField).Children(func() {
					rec.Render(c, a, false)
				})
			} else {
				ui.Text(c, "暂无推荐内容").FontSize(13 * uiScale).TextColor(clrMuted)
			}
		})
	})
}

// ---- RecommendHost 实现（依赖反转：app 提供能力，推荐插件只认接口） ----

func (a *app) Play(songID, title, cover string) {
	a.PlayRef("bilibili", songID, title, "", cover)
}

func (a *app) PlayRef(source, id, title, artist, cover string) {
	if source == "" {
		source = "bilibili"
	}
	// 当前推荐插件的列表就是点歌上下文；找不到时用单元素列表兜底。
	list := a.recommendData[a.activeRecommend]
	idx := -1
	for i, s := range list {
		if s.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		go a.playSongs([]playback.Song{{
			BVID: id, Title: title, Artist: artist, Cover: cover, Source: source, ID: id,
		}}, 0)
		return
	}
	go a.playSourceFrom(list, idx, source)
}

var _ plugin.RecommendHost = (*app)(nil)
