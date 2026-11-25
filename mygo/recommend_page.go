//go:build native

package main

// 推荐（放来源页上）：来源页空态显示推荐预览，点击 → 推荐详情页。
// 内容 + UI 全由推荐插件自绘（preview 控制行数）；宿主只渲染选中的那一个插件，
// 用下拉框选（对齐搜索栏音源下拉：一次只显示一个，不堆叠）。

import (
	"github.com/egoist/mygo/ui"
	"homecast/bilibili"
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

// recommendSelector 推荐插件下拉框（模仿 sourceSelector：一次选一个，中文、可滚动、点击区大）。
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
	sel.Trigger.Height(34 * uiScale).PaddingX(12 * uiScale).Draw(func(p *ui.Painter, r ui.Rect) {
		if sel.Trigger.Hovered() {
			p.FillRadii(r, [4]float32{8, 8, 8, 8}, clrCardHov)
		}
	}).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(6 * uiScale).Children(func() {
			ui.Text(c, cur).FontSize(13 * uiScale).Bold().TextColor(clrText)
			ui.Text(c, "▾").FontSize(11 * uiScale).TextColor(clrMuted)
		})
	})
	popupH := len(names)*44 + 12
	if popupH > 412 {
		popupH = 412
	}
	sel.Popup(func(panel ui.Element) {
		panel.Padding(6 * uiScale).Background(clrCard).Radius(12 * uiScale).Height(float32(popupH) * uiScale)
		ui.Scroll(c).Grow(1).Scrollbars(false).Children(func() {
			for _, name := range names {
				name := name
				item := sel.Item(name).Height(44 * uiScale).PaddingX(12 * uiScale)
				if item.Highlighted() {
					item.Background(clrCardHov)
				}
				item.Children(func() {
					col := clrMuted
					if name == cur {
						col = clrPink
					}
					ui.Text(c, name).FontSize(13 * uiScale).TextColor(col)
				})
			}
		})
	})
}

// recommendPreview 来源页空态的推荐预览区（下拉选一个插件，preview=true 精简展示）。
func (a *app) recommendPreview(c *ui.Context) {
	rec := a.currentRecommend()
	if rec == nil {
		return
	}
	ui.Column(c).Fill().Gap(10 * uiScale).Children(func() {
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
			ui.Text(c, "推荐").FontSize(a.resp(18, 16)).Bold().TextColor(clrText)
			ui.Spacer(c).Grow(1)
			a.recommendSelector(c)
			if chip(c, "查看全部").Clicked() {
				a.recommendDetailOpen = true
				a.win.Invalidate()
			}
		})
		section := ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(14 * uiScale).PaddingY(12 * uiScale).Background(clrField)
		section.Children(func() {
			rec.Render(c, a, true)
		})
	})
}

// recommendDetail 推荐详情页（只显示选中的插件，preview=false 完整展示）。
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
				a.recommendSelector(c)
			})
			if rec != nil {
				section := ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(14 * uiScale).PaddingY(12 * uiScale).Background(clrField)
				section.Children(func() {
					rec.Render(c, a, false)
				})
			}
		})
	})
}

// ---- RecommendHost 实现（依赖反转：app 提供能力，推荐插件只认接口） ----

// Play 播放歌曲（推荐插件调；songID=音源 id）。
func (a *app) Play(songID, title, cover string) {
	a.play(bilibili.SearchItem{BVID: songID, Title: title, Pic: cover})
}

// PlayRef 按真实来源播放（榜单等跨源推荐用）。source 是音源 key，id 是该音源内 id。
func (a *app) PlayRef(source, id, title, artist, cover string) {
	if source == "" {
		source = "bilibili"
	}
	a.play(bilibili.SearchItem{BVID: id, Title: title, Author: artist, Pic: cover, Typename: source})
}

// 编译期断言：app 实现 RecommendHost（Invalidate/Resp 已在 target_panel_host.go 声明）。
var _ plugin.RecommendHost = (*app)(nil)
