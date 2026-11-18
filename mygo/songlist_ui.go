//go:build native

// 歌单页（mygo 原生）：复用主线 songlist 域包（SSOT），UI 与主线 /hx/songlists 对齐。
// 与「队列」（= 播放列表）区分：歌单是用户自建、可命名、可多份的收藏集合。
package main

import (
	"fmt"
	"strconv"

	"github.com/egoist/mygo/ui"

	"homecast/bilibili"
	"homecast/service/songlist"
)

// songlistCoverKey 歌单封面的缓存 key（loadCover 用 bvid 作 map key，歌单没有 bvid → 前缀区分）
func songlistCoverKey(id int64) string { return "__sl_" + strconv.FormatInt(id, 10) }

// refreshSonglists 从 songlist 域包刷新歌单列表快照（UI 渲染用）。
func (a *app) refreshSonglists() {
	if a.svc == nil || a.svc.songlist == nil {
		a.songlists = nil
		return
	}
	a.songlists = a.svc.songlist.List()
}

// loadSonglistDetail 打开/刷新当前歌单详情（a.songlistOpen 为目标 id）。
func (a *app) loadSonglistDetail() {
	if a.svc == nil || a.svc.songlist == nil || a.songlistOpen == 0 {
		return
	}
	g, items, err := a.svc.songlist.Get(a.songlistOpen)
	if err != nil {
		a.songlistOpen = 0 // 歌单被删（别处/上次会话）→ 回列表
		a.songlistItems = nil
		return
	}
	a.songlistCur = g
	a.songlistItems = items
}

// songlistToSearchItems 歌单歌曲 → []bilibili.SearchItem（playAll/右键菜单复用）
func songlistToSearchItems(list []songlist.Item) []bilibili.SearchItem {
	out := make([]bilibili.SearchItem, 0, len(list))
	for _, it := range list {
		out = append(out, bilibili.SearchItem{
			BVID: it.BVID, Title: it.Title, Author: it.Artist, Pic: it.Cover, Duration: it.Duration,
		})
	}
	return out
}

// songlistPage 歌单页入口：a.songlistOpen != 0 → 详情；否则列表。
func (a *app) songlistPage(c *ui.Context) {
	if a.songlistOpen != 0 {
		a.songlistDetail(c)
		return
	}
	a.songlistGridPage(c)
}

// songlistGridPage 歌单列表页：标题 + 新建 + 网格卡片。
func (a *app) songlistGridPage(c *ui.Context) {
	ui.Scroll(c).Grow(1).Scrollbars(true).Children(func() {
		ui.Column(c).Fill().PaddingX(a.resp(24, 12)).PaddingY(a.resp(16, 10)).Gap(14 * uiScale).Children(func() {
			// 标题行
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
				ui.Text(c, "歌单").FontSize(20 * uiScale).Bold().TextColor(clrText)
				ui.Text(c, fmt.Sprintf("%d 份", len(a.songlists))).FontSize(13 * uiScale).TextColor(clrMuted)
				ui.Spacer(c).Grow(1)
				if gradButton(c, "新建歌单").Clicked() {
					a.songlistNewOpen = true
					a.songlistNameInput = ""
					a.win.Invalidate()
				}
			})
			// 新建输入行（展开态）
			if a.songlistNewOpen {
				a.songlistNewRow(c)
			}
			// 空态（且不在新建中）
			if len(a.songlists) == 0 && !a.songlistNewOpen {
				ui.Column(c).Fill().Center().Gap(12 * uiScale).Children(func() {
					icon(c, icList, 44, ui.RGBA(0xff, 0xff, 0xff, 0.08))
					ui.Text(c, "还没有歌单").FontSize(17 * uiScale).Bold().TextColor(clrText)
					ui.Text(c, "把喜欢的歌归类收藏").FontSize(13 * uiScale).TextColor(clrMuted)
				})
				return
			}
			if len(a.songlists) == 0 {
				return
			}
			// 网格卡片（列数按 tier 对应主线 grid-cols-2 sm:3 md:4 lg:5）
			cols := a.songlistGridCols()
			tracks := make([]ui.Track, cols)
			for i := range tracks {
				tracks[i] = ui.Fr(1)
			}
			ui.Grid(c).ColumnTracks(tracks...).Gap(12 * uiScale).Fill().Children(func() {
				for _, g := range a.songlists {
					a.songlistCard(c, g)
				}
			})
		})
	})
}

// songlistNewRow 新建歌单输入行（TextInput + 确定/取消）
func (a *app) songlistNewRow(c *ui.Context) {
	box := ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(14 * uiScale).PaddingY(10 * uiScale).Background(clrCard)
	box.Children(func() {
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
			ui.TextInput(c, &a.songlistNameInput).Grow(1).Height(40 * uiScale).
				Placeholder("歌单名称").TextColor(clrText)
			if gradButton(c, "确定").Clicked() {
				a.createSonglist()
			}
			if chip(c, "取消").Clicked() {
				a.songlistNewOpen = false
				a.win.Invalidate()
			}
		})
	})
}

// createSonglist 新建歌单（空名忽略；成功后回列表）
func (a *app) createSonglist() {
	if a.svc == nil || a.svc.songlist == nil {
		return
	}
	name := a.songlistNameInput
	if name == "" {
		return
	}
	if _, err := a.svc.songlist.Create(name); err != nil {
		a.status = "歌单名不能为空"
		return
	}
	a.songlistNewOpen = false
	a.songlistNameInput = ""
	a.refreshSonglists()
	a.win.Invalidate()
}

// songlistGridCols 歌单网格列数（对齐主线 grid-cols-2 sm:3 md:4 lg:5）
func (a *app) songlistGridCols() int {
	switch a.tier() {
	case 0: // 手机（<768）
		return 2
	case 1: // 紧凑（768-1024）
		return 3
	default: // 桌面（>=1024）
		return 4
	}
}

// songlistCard 歌单网格卡片：方形封面 + 名称 + 首数（对齐主线 songlist_grid.html）
func (a *app) songlistCard(c *ui.Context, g songlist.Songlist) {
	size := a.songlistCardSize()
	key := songlistCoverKey(g.ID)
	// 复用 gridCard 共享样式（封面盒 + hover 遮罩 + 标题/副标题）
	a.gridCard(c, key, g.Cover, g.Name, fmt.Sprintf("%d 首", g.Count), size, func() {
		a.songlistOpen = g.ID
		a.loadSonglistDetail()
		a.win.Invalidate()
	})
}

// songlistCardSize 歌单卡片封面边长（按 tier 自适应）
func (a *app) songlistCardSize() float32 {
	switch a.tier() {
	case 0:
		return a.resp(104, 96)
	case 1:
		return a.resp(120, 104)
	default:
		return a.resp(140, 120)
	}
}

// songlistDetail 歌单详情页：标题 + 操作 + 歌曲行。
func (a *app) songlistDetail(c *ui.Context) {
	ui.Scroll(c).Grow(1).Scrollbars(true).Children(func() {
		ui.Column(c).Fill().PaddingX(a.resp(24, 12)).PaddingY(a.resp(16, 10)).Gap(12 * uiScale).Children(func() {
			// 标题行：返回 + 歌单名 + 首数 + 操作
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Children(func() {
				if iconButton2(c, icPrev, 32, clrText).Clicked() {
					a.songlistOpen = 0
					a.songlistItems = nil
					a.refreshSonglists()
					a.win.Invalidate()
				}
				ui.Text(c, a.songlistCur.Name).FontSize(18 * uiScale).Bold().TextColor(clrText).MaxLines(1)
				ui.Text(c, fmt.Sprintf("%d 首", len(a.songlistItems))).FontSize(12 * uiScale).TextColor(clrMuted)
				ui.Spacer(c).Grow(1)
				if len(a.songlistItems) > 0 {
					if chip(c, "播放全部").Clicked() {
						a.playAll(songlistToSearchItems(a.songlistItems), 0)
					}
					if chip(c, "清空").Clicked() {
						a.svc.songlist.Clear(a.songlistOpen)
						a.loadSonglistDetail()
						a.win.Invalidate()
					}
				}
				if chip(c, "删除").Clicked() {
					_ = a.svc.songlist.Delete(a.songlistOpen)
					a.songlistOpen = 0
					a.songlistItems = nil
					a.refreshSonglists()
					a.win.Invalidate()
				}
			})
			// 改名输入行（展开态）
			if a.songlistRenameOpen {
				a.songlistRenameRow(c)
			}
			// 空态
			if len(a.songlistItems) == 0 {
				ui.Column(c).Fill().Center().Gap(12 * uiScale).Children(func() {
					icon(c, icList, 44, ui.RGBA(0xff, 0xff, 0xff, 0.08))
					ui.Text(c, "这个歌单还是空的").FontSize(17 * uiScale).Bold().TextColor(clrText)
					ui.Text(c, "在搜索/收藏页点歌曲「更多」→「添加到歌单」").FontSize(13 * uiScale).TextColor(clrMuted)
				})
				return
			}
			// 歌曲行
			ui.Column(c).Gap(6 * uiScale).Children(func() {
				for _, it := range a.songlistItems {
					a.songlistRow(c, it)
				}
			})
		})
	})
}

// songlistRenameRow 重命名输入行（展开态）
func (a *app) songlistRenameRow(c *ui.Context) {
	box := ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(14 * uiScale).PaddingY(10 * uiScale).Background(clrCard)
	box.Children(func() {
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
			ui.TextInput(c, &a.songlistRenameInput).Grow(1).Height(40 * uiScale).
				Placeholder("新歌单名称").TextColor(clrText)
			if gradButton(c, "保存").Clicked() {
				if a.svc.songlist.Rename(a.songlistOpen, a.songlistRenameInput) == nil {
					a.songlistRenameOpen = false
					a.loadSonglistDetail()
					a.refreshSonglists()
					a.win.Invalidate()
				}
			}
			if chip(c, "取消").Clicked() {
				a.songlistRenameOpen = false
				a.win.Invalidate()
			}
		})
	})
}

// songlistRow 歌单内歌曲行（对齐主线 songlist_items.html 的歌曲行）
// songlistRow 歌单详情行（复用 songRow）：✕ 从歌单移除 + 时长；单击播放 + 右键菜单。
func (a *app) songlistRow(c *ui.Context, it songlist.Item) {
	a.songRow(c, it.BVID, it.Cover, it.Title, it.Artist, it.Duration, songRowOpt{
		rmFn: func() {
			a.svc.songlist.RemoveSong(a.songlistOpen, it.BVID)
			a.loadSonglistDetail()
			a.win.Invalidate()
		},
	})
}

// addToSonglistSheet 「添加到歌单」浮层：列出全部歌单，点一个即加入（对齐主线 songlist.js 弹层）。
// 目标歌 = a.ctxItem（右键菜单点击时保留）。
func (a *app) addToSonglistSheet(c *ui.Context) {
	item := a.ctxItem
	if item == nil {
		return
	}
	// Overlay 层（mygo ui 专用置顶浮层，对齐 nowPlaying；否则被 Overlay 层盖住不置顶）
	ui.Overlay(c, func() {
		// 背景：点外部关闭
		bg := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0)
		bg.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Fill(r, ui.RGBA(0, 0, 0, 0.45), 0)
		})
		if bg.Clicked() {
			a.addToPick = false
			a.ctxItem = nil
			return
		}
		// 面板：居中卡片，列表可滚动
		pw := float32(340)
		ph := float32(420)
		if a.winH > 0 && ph > float32(a.winH)-80 {
			ph = float32(a.winH) - 80
		}
		lx := float32(a.winW)/2 - pw/2
		ly := float32(a.winH)/2 - ph/2
		if a.winW <= 0 {
			lx = 100
		}
		if a.winH <= 0 {
			ly = 100
		}
		panel := ui.Box(c).Absolute().Left(lx).Top(ly).Width(pw).Height(ph).Radius(16 * uiScale).Background(clrCard)
		panel.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Shadow(r, 20, 0, 12, 40, 0, ui.RGBA(0, 0, 0, 0.5))
			p.FillRadii(r, [4]float32{16, 16, 16, 16}, clrCard)
		})
		panel.Children(func() {
			ui.Column(c).Fill().Padding(16 * uiScale).Gap(10 * uiScale).Children(func() {
				// 标题行
				ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "添加到歌单").FontSize(16 * uiScale).Bold().TextColor(clrText)
					ui.Spacer(c).Grow(1)
					if iconButton2(c, icClose, 30, clrMuted).Clicked() {
						a.addToPick = false
						a.ctxItem = nil
					}
				})
				ui.Text(c, bilibili.CleanHTML(item.Title)).FontSize(12 * uiScale).TextColor(clrMuted).MaxLines(1)
				// 歌单列表（滚动）
				ui.Scroll(c).Grow(1).Scrollbars(true).Children(func() {
					ui.Column(c).Gap(4 * uiScale).Children(func() {
						if len(a.songlists) == 0 {
							ui.Text(c, "还没有歌单，点下方「新建」").FontSize(13 * uiScale).TextColor(clrMuted)
						}
						for _, g := range a.songlists {
							g := g
							row := ui.Box(c).Fill().Height(46 * uiScale).Radius(10 * uiScale).PaddingX(12 * uiScale)
							if row.Hovered() {
								row = row.Background(clrCardHov) // 主题自适应 hover（原硬编码白6%明亮主题看不见）
							}
							row.Children(func() {
								ui.Row(c).AlignSelf(ui.Stretch).Gap(10 * uiScale).AlignItems(ui.Center).Children(func() {
									ui.Column(c).Grow(1).Gap(1 * uiScale).Children(func() {
										ui.Text(c, g.Name).FontSize(14 * uiScale).TextColor(clrText).MaxLines(1)
										ui.Text(c, fmt.Sprintf("%d 首", g.Count)).FontSize(11 * uiScale).TextColor(clrMuted)
									})
								})
							})
							if row.Clicked() {
								a.addToSonglist(g.ID, item)
							}
						}
					})
				})
				// 新建并添加
				ui.Row(c).AlignSelf(ui.Stretch).Gap(8 * uiScale).AlignItems(ui.Center).Children(func() {
					ui.TextInput(c, &a.songlistNameInput).Grow(1).Height(38 * uiScale).
						Placeholder("新建歌单名称").TextColor(clrText)
					if gradButton(c, "新建").Clicked() {
						a.createAndAddToSonglist(item)
					}
				})
			})
		})
	})
}

// addToSonglist 加一首到指定歌单（成功后关浮层 + 刷新）
func (a *app) addToSonglist(id int64, item *bilibili.SearchItem) {
	if a.svc == nil || a.svc.songlist == nil || item == nil {
		return
	}
	_, err := a.svc.songlist.AddSong(id, songlist.Item{
		BVID:        item.BVID,
		Title:       bilibili.CleanHTML(item.Title),
		Artist:      item.Author,
		Cover:       item.Pic,
		Duration:    item.Duration,
		DurationSec: 0,
	})
	a.addToPick = false
	a.ctxItem = nil
	if err == nil {
		a.status = "已添加到歌单"
		a.refreshSonglists()
		if a.songlistOpen == id {
			a.loadSonglistDetail()
		}
	} else {
		a.status = err.Error()
	}
	a.win.Invalidate()
}

// createAndAddToSonglist 新建歌单并加入当前歌（浮层「新建」按钮）
func (a *app) createAndAddToSonglist(item *bilibili.SearchItem) {
	name := a.songlistNameInput
	if name == "" || a.svc == nil || a.svc.songlist == nil {
		return
	}
	g, err := a.svc.songlist.Create(name)
	if err != nil {
		a.status = err.Error()
		a.win.Invalidate()
		return
	}
	a.songlistNameInput = ""
	a.addToSonglist(g.ID, item)
	a.refreshSonglists()
}
