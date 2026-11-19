//go:build native

package main

import (
	"fmt"
	"homecast/bilibili"

	"github.com/egoist/mygo/ui"
)

func (a *app) queuePage(c *ui.Context) {
	// 整页滚动条：整页内容包一个 Scroll，不做组件内部滚动条（最近播放横排/队列列表不再单独 Scroll）
	ui.Scroll(c).Grow(1).Scrollbars(false).Children(func() {
		ui.Column(c).Fill().PaddingX(a.resp(24, 12)).PaddingY(a.resp(16, 10)).Gap(14 * uiScale).Children(func() {
			// 最近播放横排小卡（对齐主线 recent-view：最多 10 显示 + 清空）
			if len(a.recent) > 0 {
				ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Children(func() {
					ui.Text(c, "最近播放").FontSize(20 * uiScale).Bold().TextColor(clrText)
					ui.Text(c, fmt.Sprintf("共 %d 首", len(a.recent))).FontSize(12 * uiScale).TextColor(clrMuted)
					ui.Spacer(c).Grow(1)
					if chip(c, "查看全部").Clicked() {
						a.setTab("recent")
					}
					if chip(c, "清空").Clicked() {
						a.recent = nil
						a.saveRecent()
						a.win.Invalidate()
					}
				})
				// 最近播放横向滑动栏（超出窗口宽度可横滑，不挤行；上限 10 张与主线
				// recent.js"最多 10 显示"一致，全部 30 条在最近页）。
				// 显式高度 = 封面 88 + 标题行 + 歌手行 + 间距，避免横向容器把文字裁掉（"一半被遮" bug）。
				cardW := 96 * uiScale
				coverH := 88 * uiScale
				rowH := coverH + 20*uiScale + 16*uiScale + 8*uiScale
				ui.ScrollHorizontal(c).Height(rowH).Scrollbars(false).Gap(10 * uiScale).Children(func() {
					n := len(a.recent)
					if n > 10 {
						n = 10
					}
					for _, r := range a.recent[:n] {
						r := r
						// 卡片：方形封面为主 + 标题/歌手文字次之（对齐主线 recent.js 卡片结构）
						ui.Column(c).Width(cardW).Gap(4 * uiScale).Children(func() {
							// 封面卡片（对齐主线 recent.js：hover 缩放 1.02 + 阴影 + 标题变粉，非描边）
							box := ui.Box(c).Width(cardW).Height(coverH).Radius(12 * uiScale)
							// hover 高亮：阴影（对齐主线 group-hover:shadow-md）——
							// 不用 Scale（mygo Scale 对图片渲染有副作用，图片会变白）也不描边，
							// 图片保持原样，只加阴影 + 标题变粉（对齐主线 text-pink-400）。
							if box.Hovered() {
								box = box.Shadow(0, 4*uiScale, 16*uiScale, 0, ui.RGBA(0, 0, 0, 0.30))
							}
							box.Draw(func(p *ui.Painter, b ui.Rect) {
								p.FillRadii(b, [4]float32{12, 12, 12, 12}, clrCard)
								if img, ok := a.covers[r.BVID]; ok {
									p.Clip(b, 12, func() { p.Image(img, b, ui.Cover) })
								}
							})
							a.loadCover(r.BVID, r.Pic)
							// 单击播放（最近播放建议单击就放）：封面卡片单击即播
							if box.Clicked() {
								go a.play(r)
							}
							// 标题（主）：hover 变粉（对齐主线 group-hover:text-pink-400）
							titleCol := clrText
							if box.Hovered() {
								titleCol = clrPink
							}
							ui.Text(c, bilibili.CleanHTML(r.Title)).FontSize(12 * uiScale).TextColor(titleCol).MaxLines(1)
							ui.Text(c, r.Author).FontSize(10 * uiScale).TextColor(clrMuted).MaxLines(1)
						})
					}
				})
			}
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
				ui.Text(c, "播放队列").FontSize(20 * uiScale).Bold().TextColor(clrText)
				ui.Text(c, fmt.Sprintf("%d 首", len(a.playlist))).FontSize(13 * uiScale).TextColor(clrMuted)
				ui.Spacer(c).Grow(1)
				if len(a.playlist) > 0 {
					if chip(c, "清空队列").Clicked() {
						a.pb.ClearQueue()
					}
				}
			})
			// 上次在听（对齐原项目）
			// 上次在听栏已移除（用户要求：不占队列页空间，最近播放已够）
			if len(a.playlist) == 0 {
				ui.Column(c).Fill().Center().Gap(10 * uiScale).Children(func() {
					icon(c, icList, 44, ui.RGBA(0xff, 0xff, 0xff, 0.08))
					ui.Text(c, "队列为空").FontSize(17 * uiScale).Bold().TextColor(clrText)
					ui.Text(c, "随便点一首歌，队列会自动出现在这里").FontSize(13 * uiScale).TextColor(clrMuted)
					if gradButton(c, "去搜索").Clicked() {
						a.setTab("search")
					}
				})
				return
			}
			ui.Column(c).Gap(6 * uiScale).Children(func() {
				queuePage := a.pg["queue"]
				for i, item := range pageSlice(a.playlist, queuePage) {
					a.queueRow(c, queuePage*pageSize+i, item)
				}
				pager(c, queuePage, pageCount(len(a.playlist)), func(p int) {
					a.pg["queue"] = p
					a.win.Invalidate()
				})
			})
		})
	})
}

// recentPage 最近播放全页（对齐主线 /hx/recent：最多 30 首列表）。
func (a *app) queueRow(c *ui.Context, i int, item bilibili.SearchItem) {
	isCur := a.current != nil && a.current.bvid == item.BVID && i == a.playIndex
	idx := i
	a.songRow(c, item.BVID, item.Pic, item.Title, item.Author, "", songRowOpt{
		idx:   &idx,
		isCur: isCur,
		rmFn:  func() { a.pb.RemoveQueue(item.BVID) },
	})
}

// favPage 收藏页。
