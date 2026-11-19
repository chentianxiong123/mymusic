//go:build native

package main

// 推荐页：壳，内容 + UI 全由推荐插件自己实现。
// 宿主只遍历 plugin.Recommends() 调 Render（插件自绘），并通过 RecommendHost
// 注入能力（Play/Invalidate/Resp）。宿主不渲染任何推荐内容（一切靠插件）。
// 插件一注册 → 推荐页自动多一个区块，UI 零改动。

import (
	"github.com/egoist/mygo/ui"
	"homecast/bilibili"
	"mymusic-mygo/plugin"
)

// recommendPage 推荐页：整页滚动，遍历推荐插件（每插件自绘自己的推荐 UI）。
func (a *app) recommendPage(c *ui.Context) {
	ui.Scroll(c).Grow(1).Scrollbars(false).Children(func() {
		ui.Column(c).Fill().PaddingX(a.resp(24, 12)).PaddingY(a.resp(16, 10)).Gap(14 * uiScale).Children(func() {
			// 标题行
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
				ui.Text(c, "推荐").FontSize(20 * uiScale).Bold().TextColor(clrText)
				ui.Spacer(c).Grow(1)
			})
			recs := plugin.Recommends()
			if len(recs) == 0 {
				// 无推荐插件：提示（注册推荐插件后自动出现，内容靠插件）
				ui.Column(c).Fill().Center().Gap(10 * uiScale).Children(func() {
					icon(c, icRecommend, 44, ui.RGBA(0xff, 0xff, 0xff, 0.08))
					ui.Text(c, "暂无推荐").FontSize(17 * uiScale).Bold().TextColor(clrText)
					ui.Text(c, "注册推荐插件后，这里按插件自绘展示").FontSize(13 * uiScale).TextColor(clrMuted)
				})
				return
			}
			// 每插件一个区块：内容 + UI 全由插件 Render 自绘（宿主只提供容器 + 能力）
			for _, r := range recs {
				section := ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(14 * uiScale).PaddingY(12 * uiScale).Background(clrField)
				section.Children(func() {
					r.Render(c, a) // 插件自绘（宿主 a 实现 RecommendHost）
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
