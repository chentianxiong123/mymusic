//go:build native

package main

// 推荐（放来源页上）：来源页空态显示推荐预览（每插件 1-2 行精简，吸引点击），
// 点击 → 推荐详情页（分插件完整展示）。内容 + UI 全由推荐插件自绘（preview 控制行数）。
// 宿主只遍历调 Render + 注入能力（RecommendHost），不渲染推荐内容（一切靠插件）。

import (
	"github.com/egoist/mygo/ui"
	"homecast/bilibili"
	"mymusic-mygo/plugin"
)

// recommendPreview 来源页空态的推荐预览区（每插件 1-2 行精简 + 查看全部入口）。
// 点击推荐区 → 推荐详情页（分插件完整展示）。
func (a *app) recommendPreview(c *ui.Context) {
	recs := plugin.Recommends()
	if len(recs) == 0 {
		return // 无推荐插件：不显示推荐区（来源页只留风格分类）
	}
	ui.Column(c).Fill().Gap(14 * uiScale).Children(func() {
		// 标题行：推荐 + 查看全部（进详情页）
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
			ui.Text(c, "推荐").FontSize(a.resp(18, 16)).Bold().TextColor(clrText)
			ui.Spacer(c).Grow(1)
			if chip(c, "查看全部").Clicked() {
				a.recommendDetailOpen = true
				a.win.Invalidate()
			}
		})
		// 每插件预览区块（preview=true：插件画 1-2 行精简，吸引点击进详情）
		for _, r := range recs {
			section := ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(14 * uiScale).PaddingY(12 * uiScale).Background(clrField)
			section.Children(func() {
				r.Render(c, a, true)
			})
			// 点击推荐区块 → 推荐详情页（分插件完整展示）
			if section.Clicked() {
				a.recommendDetailOpen = true
				a.win.Invalidate()
			}
		}
	})
}

// recommendDetail 推荐详情页（分插件完整展示，preview=false 插件画全部）。
func (a *app) recommendDetail(c *ui.Context) {
	ui.Scroll(c).Grow(1).Scrollbars(false).Children(func() {
		ui.Column(c).Fill().PaddingX(a.resp(24, 12)).PaddingY(a.resp(16, 10)).Gap(14 * uiScale).Children(func() {
			// 返回 + 标题
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
				if iconButton2(c, icBack, a.resp(30, 28), clrText).Clicked() {
					a.recommendDetailOpen = false
					a.win.Invalidate()
				}
				ui.Text(c, "推荐").FontSize(20 * uiScale).Bold().TextColor(clrText)
				ui.Spacer(c).Grow(1)
			})
			// 分插件完整展示（preview=false）
			for _, r := range plugin.Recommends() {
				section := ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(14 * uiScale).PaddingY(12 * uiScale).Background(clrField)
				section.Children(func() {
					r.Render(c, a, false)
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

// 编译期断言：app 实现 RecommendHost（Invalidate/Resp 已在 target_panel_host.go 声明）。
var _ plugin.RecommendHost = (*app)(nil)
