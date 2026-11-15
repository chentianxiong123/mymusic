//go:build native

// 播放控制面板 + 队列/收藏/设置页 + 播放器控制方法（seek/volume/loop/next/prev/fav）。
package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"homecast/service/lyric"
	"homecast/service/playback"
	"homecast/service/speaker"

	"homecast/bilibili"
)

// setTab 切换导航页。
func (a *app) setTab(t string) {
	a.activeTab = t
	a.win.Invalidate()
}

// accent 高亮色：条件真→粉紫，否则灰。
func accent(b bool) ui.Color {
	if b {
		return clrPink
	}
	return clrMuted
}

// playAt 播放队列第 i 项（循环取模）：SetQueue 定位（不乱序移动）。
func (a *app) playAt(i int) {
	if len(a.playlist) == 0 {
		return
	}
	n := len(a.playlist)
	i = ((i % n) + n) % n
	a.pb.SetQueue(toSongList(a.playlist), i)
}

// next 下一首（按循环模式）。顺序走 pb.Next（SSOT 队列走位）；随机走 SetQueue 定位。
func (a *app) next() {
	if len(a.playlist) == 0 {
		return
	}
	if a.loopMode == 3 { // 随机
		a.playAt(rand.Intn(len(a.playlist)))
		return
	}
	a.pb.Next()
}

// prev 上一首（按循环模式；>3s 回到开头）。
func (a *app) prev() {
	if len(a.playlist) == 0 {
		return
	}
	if a.loopMode == 2 {
		a.playAt(rand.Intn(len(a.playlist)))
		return
	}
	if a.pb.Snapshot().Position > 3 {
		a.seek(0)
		return
	}
	a.pb.Prev()
}

// seek 按比例跳转进度（走 SSOT）。
func (a *app) seek(frac float64) {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	a.pb.Seek(frac * a.pb.Snapshot().Duration)
}

// setVolume 设置音量 0..1（MyGo 本地 audio 音量；web 语义下主线不管 web 音量）。
func (a *app) setVolume(v float64) {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	a.volume = v
	if a.mygoOut != nil {
		a.mygoOut.SetVolume(int(v * 100))
	}
	now := time.Now()
	if now.Sub(a.lastVolSave) > 500*time.Millisecond {
		a.savePrefFloat("mygo_volume", v)
		a.lastVolSave = now
	}
	a.win.Invalidate()
}

// toggleLoop 循环模式：列表循环→单曲→随机（同步 SSOT）。
func (a *app) toggleLoop() {
	a.loopMode = (a.loopMode + 1) % 4
	a.pb.SetMode(intToMode(a.loopMode))
	a.savePref("mygo_loopmode", a.loopMode)
}

// loopLabel 当前循环模式名。
func (a *app) loopLabel() string {
	switch a.loopMode {
	case 1:
		return "列表循环"
	case 2:
		return "单曲循环"
	case 3:
		return "随机播放"
	default:
		return "顺序播放"
	}
}

// isFav 当前曲目是否已收藏（SSOT：fav 域包）。
func (a *app) isFav() bool {
	return a.current != nil && a.svc.favHas(a.current.bvid)
}

// toggleFav 收藏/取消当前曲目（SSOT：fav 域包 Add/Remove）。
func (a *app) toggleFav() {
	if a.current == nil {
		return
	}
	if a.svc.favHas(a.current.bvid) {
		a.svc.fav.Remove(a.current.bvid)
	} else {
		a.svc.fav.Add(favNewEntry(a.current.bvid, a.current.title, a.current.author, a.current.pic, durationSec(0, "")))
	}
	a.refreshFavs()
	a.win.Invalidate()
}

// isFavItem 指定曲目是否已收藏（SSOT）。
func (a *app) isFavItem(item bilibili.SearchItem) bool {
	return a.svc.favHas(item.BVID)
}

// toggleFavItem 收藏/取消指定曲目（SSOT）。
func (a *app) toggleFavItem(item bilibili.SearchItem) {
	if a.svc.favHas(item.BVID) {
		a.svc.fav.Remove(item.BVID)
	} else {
		a.svc.fav.Add(favNewEntry(item.BVID, bilibili.CleanHTML(item.Title), item.Author, item.Pic, 0))
	}
	a.refreshFavs()
	a.win.Invalidate()
}

// refreshFavs 从 fav 域包刷新 favorites 快照（UI 渲染用）。
func (a *app) refreshFavs() {
	list := a.svc.fav.List()
	a.favorites = a.favorites[:0]
	a.favSet = make(map[string]bool, len(list))
	for _, f := range list {
		a.favSet[f.BVID] = true
		a.favorites = append(a.favorites, bilibili.SearchItem{
			BVID: f.BVID, Title: f.Title, Author: f.Artist, Pic: f.Cover,
			Duration: f.Duration,
		})
	}
}

// volumeBar 音量滑块：拖动 SetVolume，点击轨道定位。
func (a *app) volumeBar(c *ui.Context, width float32) ui.Element {
	box := ui.Box(c).Width(width).Height(4).Radius(2)
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		p.FillRadii(r, [4]float32{r.H / 2, r.H / 2, r.H / 2, r.H / 2}, clrTrackBar)
		v := float32(a.volume)
		if v > 0 {
			fr := ui.Rect{X: r.X, Y: r.Y, W: r.W * v, H: r.H}
			p.FillGradient(fr, grad(), r.H/2)
			p.Fill(ui.Rect{X: fr.X + fr.W - 3, Y: r.Y - 3, W: 6, H: 6}, ui.RGB(0xff, 0xff, 0xff), 3)
		}
		if x, _, over := box.PointerPosition(); over && (box.Pressed() || box.Dragging()) {
			x0 := x
			if x0 < 0 {
				x0 = 0
			}
			if x0 > r.W {
				x0 = r.W
			}
			a.setVolume(float64(x0 / r.W))
		}
		// hover 显示百分比（对齐音量条语义：可见即反馈）
		if box.Hovered() {
			pct := int(a.volume * 100)
			if a.muted {
				pct = 0
			}
			label := fmt.Sprintf("%d%%", pct)
			bub := ui.Rect{X: r.X + r.W/2 - 16, Y: r.Y - 24, W: 32, H: 18}
			p.FillRadii(bub, [4]float32{9, 9, 9, 9}, clrTrack)
			p.Text(bub.X+16, bub.Y+13, label, 10, clrText)
		}
	})
	return box
}

// nowPlaying 正在播放歌词页：对齐主线 lyric-panel 竖屏歌词面板结构
// （三栏：候选栏 + 中栏歌词/进度 + 工具列；背景封面模糊/纯色；<768 候选/工具收抽屉，歌词全宽）。
func (a *app) nowPlaying(c *ui.Context) {
	ui.Overlay(c, func() {
		bg := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0)
		bg.Draw(func(p *ui.Painter, r ui.Rect) {
			// 背景：封面模糊（默认）/ 纯色（对齐主线 ly-bg-cover / ly-bg-solid）
			if a.lyricBg == 0 {
				if img, ok := a.covers[a.current.bvid]; ok {
					p.Image(img, r, ui.Cover)
					p.Fill(r, ui.RGBA(0x0a, 0x0b, 0x10, 0.72), 0)
				} else {
					p.Fill(r, ui.RGBA(0x0e, 0x0f, 0x14, 0.97), 0)
				}
			} else {
				p.Fill(r, ui.RGBA(0x0e, 0x0f, 0x14, 0.985), 0)
			}
			p.FillGradient(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: 170},
				ui.LinearGradient{From: ui.RGBA(0x8b, 0x5c, 0xf6, 0.14), To: ui.RGBA(0xec, 0x48, 0x99, 0.0), Angle: 270}, 170)
		})
		if bg.Clicked() {
			a.showPlayer = false
		}
		// 关闭按钮（对齐主线 ly-close：右上角）
		ui.Row(c).Absolute().Top(18).Right(20).Children(func() {
			if iconButton(c, "✕").Clicked() {
				a.showPlayer = false
			}
		})

		if a.current == nil {
			ui.Column(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).Center().Children(func() {
				ui.Text(c, "未在播放").FontSize(22).Bold().TextColor(clrMuted)
			})
			return
		}

		// 未加载歌词 → 触发异步加载（对齐主线 auto-search 当前歌名）
		if len(a.lyricLines) == 0 && !a.lyricLoading {
			go a.loadLyricAsync(a.current.bvid)
		}

		// 主内容：max-w-6xl 居中三栏（对齐主线 lyric-panel h-full max-w-6xl mx-auto flex gap）
		ui.Row(c).Absolute().Left(0).Top(64).Right(0).Bottom(0).PaddingX(a.resp(24, 10)).PaddingY(a.resp(12, 6)).Gap(a.resp(16, 8)).Children(func() {
			// 「窄档」：歌词全宽单栏（候选/工具收进左右抽屉按钮，对齐主线 sm:hidden）
			if a.narrow() {
				a.narrowLyricPanel(c)
				return
			}
			// 桌面三栏
			a.lyrCandCol(c)   // 左：候选
			a.lyrCenterCol(c) // 中：信息 + KTV 歌词 + 进度
			a.lyrToolCol(c)   // 右：工具
		})
	})
}

// lyrCandCol 左栏：歌词候选（对齐主线 ly-cand-drawer：w-64 列表，点选换源）。
func (a *app) lyrCandCol(c *ui.Context) {
	box := ui.Box(c).Width(a.resp(240, 200)).Fill().Radius(16).Background(clrCard).PaddingX(10).PaddingY(8)
	box.Children(func() {
		ui.Column(c).Gap(6).Children(func() {
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "歌词候选").FontSize(13).Bold().TextColor(clrPink)
				ui.Spacer(c).Grow(1)
				ui.Text(c, a.current.title).FontSize(11).TextColor(clrMuted).MaxLines(1).MaxWidth(120)
			})
			if len(a.lyricCands) == 0 {
				ui.Text(c, LaodingTag()).FontSize(12).TextColor(clrMuted).PaddingY(20)
				return
			}
			ui.Scroll(c).Grow(1).Gap(4).Children(func() {
				ui.Column(c).Gap(4).Children(func() {
					for _, cand := range a.lyricCands {
						on := a.lyricSID == cand.ID
						row := ui.Box(c).Fill().Height(52).Radius(10).PaddingX(10)
						if on {
							row.Draw(func(p *ui.Painter, r ui.Rect) {
								p.FillGradient(r, grad(), 10)
								p.Fill(r, ui.RGBA(0x0e, 0x0f, 0x14, 0.5), 10)
							})
						} else if row.Hovered() {
							row.Background(clrField)
						}
						row.Children(func() {
							ui.Column(c).Grow(1).Gap(2).Children(func() {
								name := cand.Name
								ui.Text(c, name).FontSize(13).Bold().TextColor(clrText).MaxLines(1)
								ui.Text(c, cand.Artist).FontSize(11).TextColor(clrMuted).MaxLines(1)
							})
							if cand.Duration > 0 {
								ui.Text(c, fmt.Sprintf("%d:%02d", cand.Duration/60, cand.Duration%60)).FontSize(11).TextColor(clrMuted)
							}
						})
						if row.Clicked() {
							go a.pickLyricSID(a.current.bvid, cand.ID)
						}
					}
				})
			})
		})
	})
}

// LaodingTag 加载中文态。
func LaodingTag() string { return "搜索中…" }

// lyrCenterCol 中栏：歌曲信息 + KTV 歌词列表 + 底部进度条（对齐主线 lyric-panel 中栏）。
func (a *app) lyrCenterCol(c *ui.Context) {
	ui.Column(c).Grow(1).Fill().Gap(10).Children(func() {
		// 顶部：封面 + 标题/歌手（对齐主线 ly-cover/ly-title/ly-artist）
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(12).PaddingX(4).Children(func() {
			a.cover(c, a.current.bvid, a.current.pic, a.resp(60, 48))
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Text(c, bilibili.CleanHTML(a.current.title)).FontSize(a.resp(22, 17)).Bold().TextColor(clrText).MaxLines(1)
				ui.Text(c, a.current.author).FontSize(13).TextColor(clrMuted).MaxLines(1)
			})
			// 字幕条快捷开关（对齐主线 d-tick 语义）
			if iconButton2(c, icMsg, 34, accent(a.captionShown)).Clicked() {
				a.toggleCaption()
			}
		})
		// KTV 歌词列表（对齐主线 ly-lines：当前行高亮放大）
		ui.Box(c).Grow(1).Fill().Radius(14).Background(clrCard).PaddingX(a.resp(28, 14)).PaddingY(10).Children(func() {
			_, curIdx := a.lyricAt(a.posSec)
			ui.Scroll(c).Fill().Gap(10).Children(func() {
				ui.Column(c).AlignSelf(ui.Stretch).Gap(10).Children(func() {
					if len(a.lyricLines) == 0 {
						ui.Column(c).AlignSelf(ui.Stretch).Children(func() {
							ui.Text(c, "歌词加载中…").FontSize(15).TextColor(clrMuted).AlignSelf(ui.Center).PaddingY(40)
							ui.Text(c, "网易云词源（搜索："+lyric.CleanSearchKeyword(bilibili.CleanHTML(a.current.title))+"）").FontSize(11).TextColor(clrMuted).AlignSelf(ui.Center)
						})
						return
					}
					for i, ln := range a.lyricLines {
						i, ln := i, ln
						text := lyricLineText(ln)
						if text == "" {
							continue
						}
						col := clrMuted
						size := a.lyricFontSize()
						if i == curIdx {
							col = ui.RGB(0xff, 0xff, 0xff)
							size = a.lyricFontSize() * 1.35
						}
						if a.lyricTime && i == curIdx {
							ui.Row(c).AlignSelf(ui.Center).Gap(8).AlignItems(ui.Center).Children(func() {
								ui.Text(c, lyricTimeStr(lyricLineTime(ln))).FontSize(11).TextColor(clrPink)
								ui.Text(c, text).FontSize(size).Bold().TextColor(col)
							})
						} else {
							ui.Text(c, text).FontSize(size).Bold().TextColor(col)
						}
					}
				})
			})
		})
		// 底部进度条（对齐主线 ly-progress：可拖 + 当前/总时长）
		ui.Row(c).AlignSelf(ui.Stretch).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.posText).FontSize(12).TextColor(clrMuted)
			a.progressBarSized(c, a.progress, 6, true).Grow(1)
			ui.Text(c, a.durText).FontSize(12).TextColor(clrMuted)
		})
	})
}

// lyrToolCol 右栏：工具列（对齐主线 ly-tool-drawer：字号± / 对齐±0.5s / 刷新）。
func (a *app) lyrToolCol(c *ui.Context) {
	ui.Column(c).Width(68).Fill().Gap(8).Children(func() {
		toolCard := func(fn func()) {
			box := ui.Box(c).Fill().Radius(14).Background(clrCard).PaddingX(6).PaddingY(10)
			box.Children(fn)
		}
		toolCard(func() {
			ui.Column(c).AlignSelf(ui.Center).Gap(6).Children(func() {
				ui.Text(c, "字号").FontSize(11).TextColor(clrMuted)
				if iconButton(c, "＋").Clicked() {
					if a.lyricSize < 4 {
						a.lyricSize++
						a.win.Invalidate()
					}
				}
				ui.Text(c, []string{"14", "16", "18", "22", "26"}[a.lyricSize]).FontSize(12).TextColor(clrText)
				if iconButton(c, "－").Clicked() {
					if a.lyricSize > 0 {
						a.lyricSize--
						a.win.Invalidate()
					}
				}
			})
		})
		toolCard(func() {
			ui.Column(c).AlignSelf(ui.Center).Gap(6).Children(func() {
				ui.Text(c, "对齐").FontSize(11).TextColor(clrMuted)
				if iconButton(c, "▲").Clicked() {
					a.nudgeLyric(0.5)
				}
				ui.Text(c, fmt.Sprintf("%.1fs", a.lyricOffset)).FontSize(10).TextColor(clrPink)
				if iconButton(c, "▼").Clicked() {
					a.nudgeLyric(-0.5)
				}
			})
		})
		toolCard(func() {
			ui.Column(c).AlignSelf(ui.Center).Gap(6).Children(func() {
				if iconButton(c, "⟳").Clicked() {
					a.lyricLines = nil
					a.win.Invalidate()
					go a.loadLyricAsync(a.current.bvid)
				}
			})
		})
	})
}

// narrowLyricPanel 窄档歌词面板：全宽 KTV 歌词（对齐主线 <640 抽屉收起、歌词全宽）。
func (a *app) narrowLyricPanel(c *ui.Context) {
	ui.Row(c).Fill().Gap(a.resp(12, 8)).Children(func() {
		ui.Column(c).Grow(1).Fill().Children(func() {
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10).Children(func() {
				a.cover(c, a.current.bvid, a.current.pic, 48)
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, bilibili.CleanHTML(a.current.title)).FontSize(16).Bold().TextColor(clrText).MaxLines(1)
					ui.Text(c, a.current.author).FontSize(12).TextColor(clrMuted).MaxLines(1)
				})
				if iconButton2(c, icMsg, 32, accent(a.captionShown)).Clicked() {
					a.toggleCaption()
				}
			})
			ui.Box(c).Grow(1).Fill().Radius(12).Background(clrCard).PaddingX(12).PaddingY(10).Children(func() {
				_, curIdx := a.lyricAt(a.posSec)
				ui.Scroll(c).Fill().Gap(8).Children(func() {
					ui.Column(c).AlignSelf(ui.Stretch).Gap(8).Children(func() {
						if len(a.lyricLines) == 0 {
							ui.Text(c, "歌词加载中…").FontSize(14).TextColor(clrMuted).AlignSelf(ui.Center).PaddingY(30)
							return
						}
						for i, ln := range a.lyricLines {
							i, ln := i, ln
							text := lyricLineText(ln)
							if text == "" {
								continue
							}
							col := clrMuted
							size := a.lyricFontSize()
							if i == curIdx {
								col = ui.RGB(0xff, 0xff, 0xff)
								size = a.lyricFontSize() * 1.3
							}
							ui.Text(c, text).FontSize(size).Bold().TextColor(col)
						}
					})
				})
			})
			ui.Row(c).AlignSelf(ui.Stretch).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.posText).FontSize(12).TextColor(clrMuted)
				a.progressBarSized(c, a.progress, 5, true).Grow(1)
				ui.Text(c, a.durText).FontSize(12).TextColor(clrMuted)
			})
		})
	})
}

// queuePage 队列页：最近播放 + 上次在听 + 播放列表（对齐主线 last-view/recent-view/queue-view）。
func (a *app) queuePage(c *ui.Context) {
	ui.Column(c).Fill().PaddingX(a.resp(24, 12)).PaddingY(a.resp(16, 10)).Gap(14).Children(func() {
		// 最近播放横排小卡（对齐主线 recent-view：最多 10 显示 + 清空）
		if len(a.recent) > 0 {
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, "最近播放").FontSize(16).Bold().TextColor(clrText)
				ui.Text(c, fmt.Sprintf("共 %d 首", len(a.recent))).FontSize(12).TextColor(clrMuted)
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
			ui.Scroll(c).AlignSelf(ui.Stretch).Gap(10).Children(func() {
				ui.Row(c).Gap(10).Children(func() {
					n := len(a.recent)
					if n > 10 {
						n = 10
					}
					for _, r := range a.recent[:n] {
						r := r
						box := ui.Box(c).Width(88).Height(88).Radius(12)
						box.Draw(func(p *ui.Painter, b ui.Rect) {
							p.FillRadii(b, [4]float32{12, 12, 12, 12}, clrCard)
							if img, ok := a.covers[r.BVID]; ok {
								p.Clip(b, 12, func() { p.Image(img, b, ui.Cover) })
							} else {
								p.Clip(b, 12, func() { p.Image(img, b, ui.Cover) })
							}
						})
						a.loadCover(r.BVID, r.Pic)
						if box.Clicked() {
							go a.play(r)
						}
						if box.Hovered() {
							box.Draw(func(p *ui.Painter, b ui.Rect) {
								p.FillRadii(b, [4]float32{12, 12, 12, 12}, ui.RGBA(0, 0, 0, 0.25))
							})
						}
					}
				})
			})
		}
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Text(c, "播放队列").FontSize(20).Bold().TextColor(clrText)
			ui.Text(c, fmt.Sprintf("%d 首", len(a.playlist))).FontSize(13).TextColor(clrMuted)
			ui.Spacer(c).Grow(1)
			if len(a.playlist) > 0 {
				if chip(c, "清空队列").Clicked() {
					a.pb.ClearQueue()
				}
			}
		})
		// 上次在听（对齐原项目）
		if a.current != nil {
			ui.Box(c).Fill().Radius(16).PaddingX(18).PaddingY(16).Background(clrCard).Children(func() {
				ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(16).Children(func() {
					a.cover(c, a.current.bvid, a.current.pic, a.resp(64, 48))
					ui.Column(c).Grow(1).Gap(4).Children(func() {
						ui.Text(c, "上次在听").FontSize(12).TextColor(clrMuted)
						ui.Text(c, a.current.title).FontSize(17).Bold().TextColor(clrText).MaxLines(1)
						ui.Text(c, a.current.author).FontSize(13).TextColor(clrMuted).MaxLines(1)
					})
					if gradButton(c, "继续播放").Clicked() {
						go a.play(*a.currentToItem())
					}
				})
			})
		}
		if len(a.playlist) == 0 {
			ui.Column(c).Fill().Center().Gap(10).Children(func() {
				icon(c, icList, 44, ui.RGBA(0xff, 0xff, 0xff, 0.08))
				ui.Text(c, "队列为空").FontSize(17).Bold().TextColor(clrText)
				ui.Text(c, "随便点一首歌，队列会自动出现在这里").FontSize(13).TextColor(clrMuted)
				if gradButton(c, "去搜索").Clicked() {
					a.setTab("search")
				}
			})
			return
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(6).Children(func() {
				for i, item := range a.playlist {
					a.queueRow(c, i, item)
				}
			})
		})
	})
}

// recentPage 最近播放全页（对齐主线 /hx/recent：最多 30 首列表）。
func (a *app) recentPage(c *ui.Context) {
	ui.Column(c).Fill().PaddingX(a.resp(24, 12)).PaddingY(a.resp(16, 10)).Gap(14).Children(func() {
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Text(c, "最近播放").FontSize(20).Bold().TextColor(clrText)
			ui.Text(c, fmt.Sprintf("共 %d 首", len(a.recent))).FontSize(13).TextColor(clrMuted)
			ui.Spacer(c).Grow(1)
			if len(a.recent) > 0 {
				if chip(c, "清空").Clicked() {
					a.recent = nil
					a.saveRecent()
					a.win.Invalidate()
				}
			}
		})
		if len(a.recent) == 0 {
			ui.Column(c).Fill().Center().Gap(10).Children(func() {
				icon(c, icList, 44, ui.RGBA(0xff, 0xff, 0xff, 0.08))
				ui.Text(c, "还没有播放记录").FontSize(17).Bold().TextColor(clrText)
				ui.Text(c, "听过的歌会出现在这里，最多保留 30 首").FontSize(13).TextColor(clrMuted)
				if gradButton(c, "去搜索").Clicked() {
					a.setTab("search")
				}
			})
			return
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(6).Children(func() {
				for _, item := range a.recent {
					a.listRow(c, item)
				}
			})
		})
	})
}

// currentToItem 把当前曲目转回 SearchItem（供 play 复用）。
func (a *app) currentToItem() *bilibili.SearchItem {
	if a.current == nil {
		return nil
	}
	return &bilibili.SearchItem{BVID: a.current.bvid, Title: a.current.title, Author: a.current.author, Pic: a.current.pic}
}

// queueRow 队列行：序号 + 封面 + 标题/作者 + 当前播放高亮。
func (a *app) queueRow(c *ui.Context, i int, item bilibili.SearchItem) {
	isCur := a.current != nil && a.current.bvid == item.BVID && i == a.playIndex
	box := ui.Box(c).Fill().Height(64).Radius(12).PaddingX(12).Gap(12)
	if isCur {
		box.Draw(func(p *ui.Painter, r ui.Rect) {
			p.FillGradient(r, grad(), 12)
			p.Fill(r, ui.RGBA(0x0e, 0x0f, 0x14, 0.55), 12)
		})
	} else {
		box.Background(clrCard)
	}
	box.Children(func() {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, fmt.Sprintf("%02d", i+1)).FontSize(13).TextColor(clrMuted).Width(26)
			if btn, ok := box.PressedButton(); ok && btn == 2 {
				it := item
				a.openCtxMenu(&it)
			}
			a.cover(c, item.BVID, item.Pic, a.resp(44, 36))
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Text(c, bilibili.CleanHTML(item.Title)).FontSize(a.resp(14, 13)).Bold().TextColor(clrText).MaxLines(1)
				ui.Text(c, item.Author).FontSize(12).TextColor(clrMuted).MaxLines(1)
			})
			if isCur {
				ui.Text(c, "▶ 播放中").FontSize(12).TextColor(clrPink).Bold()
			}
			if iconButton(c, "✕").Clicked() {
				a.pb.RemoveQueue(item.BVID)
			}
		})
	})
	if box.Clicked() {
		a.playAt(i)
	}
}

// favPage 收藏页。
func (a *app) favPage(c *ui.Context) {
	ui.Column(c).Fill().PaddingX(a.resp(24, 12)).PaddingY(a.resp(16, 10)).Gap(14).Children(func() {
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Text(c, "我的收藏").FontSize(20).Bold().TextColor(clrText)
			ui.Text(c, fmt.Sprintf("%d 首", len(a.favorites))).FontSize(13).TextColor(clrMuted)
			ui.Spacer(c).Grow(1)
			if len(a.favorites) > 0 {
				if gradButton(c, "播放全部").Clicked() {
					a.playAll(a.favorites, 0)
				}
				if chip(c, "清空收藏").Clicked() {
					a.svc.fav.Clear()
					a.refreshFavs()
					a.win.Invalidate()
				}
			}
		})
		if len(a.favorites) == 0 {
			ui.Column(c).Fill().Center().Gap(12).Children(func() {
				icon(c, icHeart, 44, ui.RGBA(0xff, 0xff, 0xff, 0.08))
				ui.Text(c, "收藏夹还是空的").FontSize(17).Bold().TextColor(clrText)
				ui.Text(c, "听歌时点卡片上的 ♥ 收藏").FontSize(13).TextColor(clrMuted)
				if gradButton(c, "去搜索").Clicked() {
					a.setTab("search")
				}
			})
			return
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(6).Children(func() {
				for _, item := range a.favorites {
					a.favRow(c, item)
				}
			})
		})
	})
}

// favRow 收藏行。
func (a *app) favRow(c *ui.Context, item bilibili.SearchItem) {
	box := ui.Box(c).Fill().Height(64).Radius(12).PaddingX(12).Background(clrCard)
	box.Children(func() {
		if btn, ok := box.PressedButton(); ok && btn == 2 {
			it := item
			a.openCtxMenu(&it)
		}
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			a.cover(c, item.BVID, item.Pic, a.resp(44, 36))
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Text(c, item.Title).FontSize(a.resp(14, 13)).Bold().TextColor(clrText).MaxLines(1)
				ui.Text(c, item.Author).FontSize(12).TextColor(clrMuted).MaxLines(1)
			})
			if iconButton2(c, icHeart, 32, clrPink).Clicked() {
				a.svc.fav.Remove(item.BVID)
				a.refreshFavs()
				a.win.Invalidate()
			}
		})
	})
	if box.Clicked() {
		go a.play(item)
	}
}

// settingsPage 设置页：全量对齐主线 settings.js 渲染的 11 个区块
// （主题模式/网页字幕条/音质/播放模式/歌词字号/歌词背景/歌词时间/字幕字号/歌曲缓存/歌词缓存/数据/关于）。
func (a *app) settingsPage(c *ui.Context) {
	ui.Column(c).Fill().PaddingX(a.resp(24, 12)).PaddingY(a.resp(16, 10)).Gap(10).Children(func() {
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "设置").FontSize(20).Bold().TextColor(clrText)
			ui.Spacer(c).Grow(1)
			ui.Text(c, "自动 ／ 明亮 ／ 暗黑（点击切换）").FontSize(12).TextColor(clrMuted)
		})
		ui.Scroll(c).Grow(1).Gap(10).Children(func() {
			ui.Column(c).Gap(10).Children(func() {
				// 1 主题模式（主线 THEMES 三态：自动/明亮/暗黑；持久化 settings kv）
				a.setRow(c, "主题模式")
				a.setRowSeg(c, []string{"自动", "明亮", "暗黑"}, a.themeMode, func(i int) {
					a.themeMode = i
					a.savePref("mygo_theme", i)
					a.win.Invalidate()
				})
				// 2 网页字幕条（主线 set-sub 开关）
				a.setRowSwitch(c, "网页字幕条", "默认关，可打开；桌面端歌词由桌面挂件优先承担", a.captionShown, func(on bool) {
					a.captionShown = on
				})
				// 3 音质（主线 d-quality / set-q 三档）
				a.setRow(c, "音质")
				a.setRowSeg(c, []string{"64k", "128k", "192k"}, a.quality, func(i int) {
					a.quality = i
					a.savePref("mygo_quality", i)
					a.win.Invalidate()
				})
				// 4 播放模式（主线 MODES 四态 + 当前模式名）
				a.setRow(c, "播放模式")
				a.setRowText(c, "当前："+a.loopLabel()+"（点按钮切换）")
				a.setRowSeg(c, []string{"顺序播放", "列表循环", "单曲循环", "随机播放"}, a.loopMode, func(i int) {
					a.loopMode = i
					a.pb.SetMode(intToMode(i))
				})
				// 5 歌词字号（主线 LY_SIZES 14/16/18/22/26）
				a.setRow(c, "歌词字号")
				a.setRowSeg(c, []string{"14px", "16px", "18px", "22px", "26px"}, a.lyricSize, func(i int) {
					a.lyricSize = i
					a.win.Invalidate()
				})
				// 6 歌词背景（主线 set-lybg：封面模糊/纯色）
				a.setRow(c, "歌词背景")
				a.setRowSeg(c, []string{"封面模糊", "纯色"}, a.lyricBg, func(i int) {
					a.lyricBg = i
					a.win.Invalidate()
				})
				// 7 歌词时间显示（主线 set-lytime 开关）
				a.setRowSwitch(c, "歌词时间显示", "歌词行旁显示时间戳", a.lyricTime, func(on bool) {
					a.lyricTime = on
					a.win.Invalidate()
				})
				// 8 字幕字号（主线 SUB_SIZES 16..32 + 当前字号说明）
				a.setRow(c, "字幕字号")
				a.setRowText(c, "当前字号："+a.capSizeLabel()+"px · 拖动字幕条可移动位置")
				a.setRowSeg(c, []string{"16px", "20px", "24px", "28px", "32px"}, a.capSize, func(i int) {
					a.capSize = i
					a.win.Invalidate()
				})
				// 9 歌曲缓存（主线 cache.js：已用/清空未收藏/清空缓存/上限）
				a.setRow(c, "歌曲缓存")
				a.setRowText(c, "自动缓存播放超过 60 秒的歌，再次播放秒开；超上限自动淘汰最旧")
				a.setRowText(c, "已用："+a.cacheUsedLabel())
				a.setRowBtns(c, []string{"清空未收藏", "清空缓存"}, func(i int) {
					if i == 0 {
						a.clearCache(false)
					} else {
						a.clearCache(true)
					}
				})
				a.setRow(c, "缓存上限")
				a.setRowSeg(c, []string{"关闭", "200MB", "500MB", "1GB", "2GB"}, a.cacheLimit, func(i int) {
					a.cacheLimit = i
					a.savePref("mygo_cachelimit", i)
					if a.svc != nil && a.svc.cache != nil {
						a.svc.cache.SetLimitMB([]int{0, 200, 500, 1000, 2000}[i])
					}
					a.win.Invalidate()
				})
				// 10 歌词缓存（主线 lyc：记忆歌词源，LRU 50 首）
				a.setRow(c, "歌词缓存")
				a.setRowText(c, "字幕条与歌词面板共用：自动记忆你选的歌词源，最多 50 首")
				a.setRowText(c, "已缓存："+a.lyricCacheCount()+" 首 · "+a.lyricCacheSize())
				a.setRowBtns(c, []string{"清空歌词缓存"}, func(i int) {
					if a.svc != nil && a.svc.lyricCache != nil {
						a.svc.lyricCache.ClearAll()
						a.win.Invalidate()
					}
				})
				// 11 数据（主线：搜索历史 + 清空）
				a.setRow(c, "数据")
				a.setRowText(c, "搜索历史（"+a.historyCount()+" 条）")
				a.setRowBtns(c, []string{"清空历史"}, func(i int) {
					a.searchHistory = nil
					a.saveHistory()
					a.win.Invalidate()
				})
				// 12 关于（主线）
				a.setRow(c, "关于")
				a.setRowText(c, "mymusic MyGo 原生版 · 紫粉渐变主题 · SSOT 共享主线 playback 域")
			})
		})
	})
}

// █ 设置页区块行（对齐主线 settings.js 的 card 卡片布局）

// setRow 区块标题行。
func (a *app) setRow(c *ui.Context, label string) {
	ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).PaddingX(4).Children(func() {
		ui.Text(c, label).FontSize(15).Bold().TextColor(clrText)
		ui.Spacer(c).Grow(1)
	})
}

// setRowText 区块内说明文字行（对齐主线 text-xs text-gray-500）。
func (a *app) setRowText(c *ui.Context, txt string) {
	ui.Text(c, txt).FontSize(12).TextColor(clrMuted).PaddingX(4)
}

// setRowSeg 分段按钮行（对齐主线 flex gap-2 圆角按钮组）。
func (a *app) setRowSeg(c *ui.Context, opts []string, cur int, cb func(int)) {
	ui.Row(c).Gap(a.resp(10, 8)).PaddingX(4).Wrap().Children(func() {
		for i, opt := range opts {
			i, opt := i, opt
			if segBtn(c, opt, i == cur).Clicked() {
				cb(i)
			}
		}
	})
}

// setRowSwitch 开关行（对齐主线 w-11 h-6 滑动开关）。
func (a *app) setRowSwitch(c *ui.Context, label, hint string, on bool, cb func(bool)) {
	ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10).PaddingX(4).Children(func() {
		ui.Text(c, label).FontSize(14).Bold().TextColor(clrText)
		ui.Spacer(c).Grow(1)
		if switchToggle(c, on).Clicked() {
			cb(!on)
		}
	})
	ui.Text(c, hint).FontSize(11).TextColor(clrMuted).PaddingX(4)
}

// setRowBtns 操作按钮行（对齐主线 清空xxx 描边小按钮）。
func (a *app) setRowBtns(c *ui.Context, labels []string, cb func(int)) {
	ui.Row(c).Gap(8).PaddingX(4).Children(func() {
		for i, l := range labels {
			i, l := i, l
			if chip(c, l).Clicked() {
				cb(i)
			}
		}
	})
}

// lyricFontSize 歌词字号当前档（对齐主线 LY_SIZES 14/16/18/22/26）。
func (a *app) lyricFontSize() float32 {
	return float32([]int{14, 16, 18, 22, 26}[a.lyricSize])
}

// capFontSize 字幕条字号当前档（对齐主线 SUB_SIZES 16/20/24/28/32）。
func (a *app) capFontSize() float32 {
	return float32([]int{16, 20, 24, 28, 32}[a.capSize])
}

// capSizeLabel 字幕字号档对应的像素值。
func (a *app) capSizeLabel() string {
	return []string{"16", "20", "24", "28", "32"}[a.capSize]
}

// cacheUsedLabel 缓存占用（SSOT：cache 域包 Size）。
func (a *app) cacheUsedLabel() string {
	if a.svc == nil || a.svc.cache == nil {
		return "0.0 MB"
	}
	mb := a.svc.cache.Size()
	return fmt.Sprintf("%.1f MB", mb)
}

// clearCache 清空缓存（SSOT：cache 域包；true=全部 ClearAll，false=仅未收藏 ClearUnfav）。
func (a *app) clearCache(all bool) {
	if a.svc == nil || a.svc.cache == nil {
		return
	}
	if all {
		a.svc.cache.ClearAll()
	} else {
		a.svc.cache.ClearUnfav("favs") // favs 表名（fav 域包常量；保留收藏的缓存）
	}
	a.win.Invalidate()
}

// lyricCacheSize 歌词缓存占用。
func (a *app) lyricCacheSize() string {
	if a.svc == nil || a.svc.lyricCache == nil {
		return "0 B"
	}
	n := a.svc.lyricCache.SizeBytes()
	if n < 1<<20 {
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
}

// lyricCacheCount 歌词缓存计数（SSOT：lyriccache 域包）。
func (a *app) lyricCacheCount() string {
	if a.svc == nil || a.svc.lyricCache == nil {
		return "0"
	}
	return fmt.Sprintf("%d", a.svc.lyricCache.Count())
}

// historyCount 搜索历史条数。
func (a *app) historyCount() string {
	return fmt.Sprintf("%d", len(a.searchHistory))
}

// settingRow 设置项行（左标签 + 右文案，对齐主线 settings 行布局）。
func (a *app) settingRow(c *ui.Context, label, value, hint string) {
	ui.Box(c).Fill().Height(56).Radius(12).PaddingX(16).Background(clrCard).Children(func() {
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Children(func() {
			ui.Text(c, label).FontSize(a.resp(15, 14)).TextColor(clrText).Bold().Width(a.resp(140, 104))
			ui.Text(c, value).FontSize(13).TextColor(clrMuted)
			ui.Spacer(c).Grow(1)
			if hint != "" {
				ui.Text(c, hint).FontSize(12).TextColor(clrMuted)
			}
		})
	})
}

// settingRowSwitch 开关行（对齐主线网页字幕条开关）。
func (a *app) settingRowSwitch(c *ui.Context, label string, on bool, hint string, cb func(bool)) {
	ui.Box(c).Fill().Height(56).Radius(12).PaddingX(16).Background(clrCard).Children(func() {
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Children(func() {
			ui.Text(c, label).FontSize(a.resp(15, 14)).TextColor(clrText).Bold().Width(a.resp(140, 104))
			ui.Text(c, hint).FontSize(13).TextColor(clrMuted)
			ui.Spacer(c).Grow(1)
			if switchToggle(c, on).Clicked() {
				cb(!on)
			}
		})
	})
}

// settingRowSeg 分段选择行（对齐主线 settings 的音质/播放模式按钮组）。
func (a *app) settingRowSeg(c *ui.Context, label string, opts []string, cur int, cb func(int)) {
	ui.Box(c).Fill().Radius(12).PaddingX(16).PaddingY(10).Background(clrCard).Children(func() {
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, label).FontSize(a.resp(15, 14)).TextColor(clrText).Bold().Width(a.resp(140, 104))
			ui.Row(c).Gap(6).Children(func() {
				for i, opt := range opts {
					i, opt := i, opt
					if segBtn(c, opt, i == cur).Clicked() {
						cb(i)
					}
				}
			})
		})
	})
}

// switchToggle 开关（开=紫高亮）。
func switchToggle(c *ui.Context, on bool) ui.Element {
	box := ui.Box(c).Size(46, 26).Radius(13)
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		if on {
			p.FillGradient(r, grad(), 13)
		} else {
			p.Fill(r, clrField, 13)
		}
	})
	box.Children(func() {
		knob := ui.Box(c).Size(20, 20).Radius(10).Absolute().Top(3)
		if on {
			knob.Left(23)
		} else {
			knob.Left(3)
		}
		knob.Background(ui.RGB(0xff, 0xff, 0xff))
	})
	return box
}

// segBtn 分段按钮（选中=渐变胶囊）。
func segBtn(c *ui.Context, label string, on bool) ui.Element {
	box := ui.Box(c).Height(32).Radius(16).PaddingX(12)
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		if on {
			p.FillGradient(r, grad(), 16)
		} else if box.Hovered() {
			p.Fill(r, clrField, 16)
		} else {
			p.Fill(r, clrField, 16)
		}
	})
	box.Children(func() {
		col := clrMuted
		if on {
			col = ui.RGB(0xff, 0xff, 0xff)
		}
		ui.Text(c, label).FontSize(13).TextColor(col)
	})
	return box
}

// castPage 投屏/音箱页：DLNA 设备发现 + 投当前歌 + 小米音箱（SSOT：cast/speaker 域包）。
func (a *app) castPage(c *ui.Context) {
	ui.Column(c).Fill().PaddingX(a.resp(24, 12)).PaddingY(a.resp(16, 10)).Gap(12).Children(func() {
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Text(c, "投屏 / 音箱").FontSize(20).Bold().TextColor(clrText)
			ui.Spacer(c).Grow(1)
			if gradButton(c, "发现设备").Clicked() {
				go a.discoverCast()
			}
		})
		// 当前投屏状态（对齐主线 d-stop：投屏中可停止）
		if a.castOn != "" {
			ui.Box(c).Fill().Radius(12).PaddingX(16).PaddingY(12).Background(clrCard).Children(func() {
				ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(12).Children(func() {
					icon(c, icCast, 18, clrPink)
					ui.Text(c, "正在投屏："+a.castOn).FontSize(14).TextColor(clrText).Bold()
					ui.Spacer(c).Grow(1)
					if chip(c, "停止投屏").Clicked() {
						a.stopCast()
					}
				})
			})
		}
		// 小米音箱（SSOT：xiaoai auth 登录态；未登录→扫码；已登录→账户头+刷新+导出 token）
		loggedIn := a.svc != nil && a.svc.speakerLoggedIn()
		if loggedIn {
			ui.Box(c).Fill().Radius(12).PaddingX(16).PaddingY(12).Background(clrCard).Children(func() {
				ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10).Children(func() {
					icon(c, icMsg, 18, clrPurple)
					ui.Column(c).Grow(1).Gap(2).Children(func() {
						ui.Text(c, "小爱音箱").FontSize(14).Bold().TextColor(clrText)
						uid := a.svc.speakerUserID()
						ui.Text(c, "已登录 · "+uid).FontSize(11).TextColor(clrMuted)
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
		} else {
			if a.qrImage != "" {
				ui.Box(c).Fill().Radius(12).PaddingX(16).PaddingY(12).Background(clrCard).Children(func() {
					ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(16).Children(func() {
						// 二维码图（UI 解码 data URI）
						ui.Box(c).Size(120, 120).Draw(func(p *ui.Painter, r ui.Rect) {
							if img, ok := a.covers["__qr__"]; ok {
								p.Clip(r, 8, func() { p.Image(img, r, ui.Cover) })
							}
							p.FillRadii(r, [4]float32{8, 8, 8, 8}, clrCard)
						})
						ui.Column(c).Grow(1).Gap(6).Children(func() {
							ui.Text(c, "小米音箱扫码登录").FontSize(14).Bold().TextColor(clrText)
							ui.Text(c, "用米家 App 扫描二维码登录").FontSize(12).TextColor(clrMuted)
							ui.Text(c, a.qrStatus).FontSize(12).TextColor(clrPink)
						})
					})
				})
			}
			ui.Box(c).Fill().Radius(12).PaddingX(16).PaddingY(10).Background(clrCard).Children(func() {
				ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10).Children(func() {
					icon(c, icMsg, 18, clrMuted)
					ui.Text(c, "小爱音箱未登录").FontSize(13).TextColor(clrText).Grow(1)
					if gradButton(c, "扫码登录").Clicked() {
						go a.startSpeakerLogin()
					}
				})
			})
		}
		// 音箱设备列表（登录后展示，SSOT speaker）
		if loggedIn {
			if len(a.speakers) == 0 {
				ui.Text(c, "音箱：点「刷新设备」获取设备列表").FontSize(12).TextColor(clrMuted).PaddingX(4)
			}
			for _, sp := range a.speakers {
				sp := sp
				row := ui.Box(c).Fill().Height(56).Radius(12).PaddingX(14).Background(clrCard)
				row.Children(func() {
					ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(12).Children(func() {
						icon(c, icVol, 18, clrMuted)
						ui.Column(c).Grow(1).Gap(2).Children(func() {
							ui.Text(c, sp.Name).FontSize(14).Bold().TextColor(clrText).MaxLines(1)
							ui.Text(c, sp.DID).FontSize(11).TextColor(clrMuted)
						})
						if gradButton(c, "播放").Clicked() {
							go a.playToSpeaker(sp.DID, sp.Name)
						}
					})
				})
			}
		}
		ui.Text(c, "DLNA 设备").FontSize(14).Bold().TextColor(clrText).PaddingX(4)
		if len(a.castDevices) == 0 {
			ui.Text(c, "点「发现设备」搜索同网段 DLNA 播放器/电视").FontSize(12).TextColor(clrMuted).PaddingX(4)
		}
		ui.Scroll(c).Grow(1).Gap(6).Children(func() {
			ui.Column(c).Gap(8).Children(func() {
				for _, d := range a.castDevices {
					d := d
					row := ui.Box(c).Fill().Height(56).Radius(12).PaddingX(14).Background(clrCard)
					if row.Hovered() {
						row.Background(clrCardHov)
					}
					row.Children(func() {
						ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(12).Children(func() {
							icon(c, icCast, 18, clrMuted)
							ui.Column(c).Grow(1).Gap(2).Children(func() {
								ui.Text(c, d.Name).FontSize(14).Bold().TextColor(clrText).MaxLines(1)
								ui.Text(c, d.IP+":"+fmt.Sprintf("%d", d.Port)).FontSize(11).TextColor(clrMuted)
							})
							if gradButton(c, "投屏").Clicked() {
								go a.castTo(d.UDN, d.Name)
							}
						})
					})
				}
			})
		})
	})
}

// startSpeakerLogin 生成音箱登录二维码（SSOT：xiaoai QRLogin）+ 轮询状态。
func (a *app) startSpeakerLogin() {
	if a.svc == nil || a.svc.qr == nil {
		return
	}
	res, err := a.svc.qr.GenerateQRCode()
	if err != nil {
		a.win.Update(func() { a.qrStatus = "二维码生成失败：" + err.Error() })
		return
	}
	a.win.Update(func() {
		qri, _ := res["qr_image"].(string)
		a.qrImage = qri
		if qri != "" {
			if img, derr := ui.DecodeImage([]byte(qri)); derr == nil {
				a.covers["__qr__"] = img
			}
		}
		a.qrStatus = "等待扫码…"
		a.win.Invalidate()
	})
	// 轮询（CheckStatus 长轮询 120s）；成功后刷新设备
	go func() {
		for {
			st, err := a.svc.qr.CheckStatus()
			if err != nil {
				a.win.Update(func() { a.qrStatus = "登录失败：" + err.Error() })
				return
			}
			status, _ := st["status"].(string)
			msg, _ := st["message"].(string)
			a.win.Update(func() { a.qrStatus = msg })
			switch status {
			case "success":
				// 把 QR token 灌进 speaker auth（对齐主线 hx/speaker.go applyQRToken）
				a.svc.applyQRToken()
				a.win.Update(func() { a.qrStatus = "登录成功，正在刷新设备…" })
				a.discoverCast()
				// 已登录 → 刷新音箱设备
				if a.svc.speakerLoggedIn() {
					go a.refreshSpeakers()
				}
				return
			case "failed":
				return
			}
		}
	}()
}

// refreshSpeakers 刷新小米音箱设备列表（speaker 域包 ListDevices）。
func (a *app) refreshSpeakers() {
	if a.svc == nil || a.svc.speaker == nil || !a.svc.speakerLoggedIn() {
		return
	}
	list := a.svc.speaker.ListDevices() // SpeakerService.ListDevices 无错误返回（auth 层处理）
	a.win.Update(func() {
		a.speakers = list
		a.win.Invalidate()
	})
}

// discoverCast 发现 DLNA 设备（cast 域包 Discover，后台执行）。
func (a *app) discoverCast() {
	if a.svc == nil || a.svc.cast == nil {
		return
	}
	_ = a.svc.cast.Discover()
	list := a.svc.cast.DeviceList()
	a.win.Update(func() {
		a.castDevices = list
		// 小米音箱设备也拉一次（登录后才有）
		if a.svc.speaker != nil {
			a.speakers = a.svc.speaker.ListDevices()
		}
		a.win.Invalidate()
	})
}

// castTo 投当前播放到指定 DLNA 设备（cast 域包 Cast）。
func (a *app) castTo(udn, name string) {
	if a.svc == nil || a.svc.cast == nil || a.current == nil {
		return
	}
	url := "https://www.bilibili.com/video/" + a.current.bvid
	res, err := a.svc.cast.Cast(url, udn, a.current.title)
	if err != nil {
		a.status = "投屏失败：" + err.Error()
		a.win.Invalidate()
		return
	}
	if code, _ := res["code"].(int); code == 0 {
		a.castOn = name
		a.status = "已投屏到 " + name
	} else {
		if msg, _ := res["message"].(string); msg != "" {
			a.status = "投屏失败：" + msg
		}
	}
	a.win.Invalidate()
}

// stopCast 停止投屏（DLNA Stop 动作）。
func (a *app) stopCast() {
	if a.svc == nil || a.svc.cast == nil {
		return
	}
	for _, d := range a.castDevices {
		a.svc.cast.Control(d.UDN, "stop", "", 0)
	}
	a.castOn = ""
	a.status = "已停止投屏"
	a.win.Invalidate()
}

// playToSpeaker 播放当前歌到小米音箱（SSOT speaker.Play：B站 bvid → DASH → 代理 URL）。
func (a *app) playToSpeaker(did, name string) {
	if a.svc == nil || a.svc.speaker == nil || a.current == nil {
		return
	}
	opts := speaker.PlayOpts{Title: a.current.title, Duration: 0}
	if res, err := a.svc.speaker.Play(a.current.bvid, did, 192, opts); err != nil {
		a.status = "音箱播放失败：" + err.Error()
	} else if code, _ := res["code"].(int); code == 0 {
		a.castOn = name
		a.status = "已在音箱播放：" + name
	}
	a.win.Invalidate()
}

// openCtxMenu 打开右键菜单（对齐主线 contextmenu：点某行）
func (a *app) openCtxMenu(item *bilibili.SearchItem) {
	a.ctxItem = item
	a.win.Invalidate()
}

// ctxMenu 右键菜单浮层（对齐主线 contextmenu.js 四项：播放/下一首播放/收藏/复制链接）
func (a *app) ctxMenu(c *ui.Context) {
	item := *a.ctxItem
	// 背景：点外部关闭
	bg := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0)
	bg.Draw(func(p *ui.Painter, r ui.Rect) {})
	if bg.Clicked() {
		a.ctxItem = nil
		return
	}
	items := []struct {
		label string
		fn    func()
	}{
		{"播放", func() { go a.play(item); a.ctxItem = nil }},
		{"下一首播放", func() { a.playNext(item); a.ctxItem = nil }},
		{"收藏 / 取消收藏", func() { a.toggleFavItem(item); a.ctxItem = nil }},
		{"复制链接", func() { a.copyLink(item); a.ctxItem = nil }},
	}
	w := float32(160)
	h := float32(len(items))*38 + 8
	// 居中定位（原生无全局鼠标坐标；用窗口尺寸居中偏下）
	lx := float32(a.winW)/2 - w/2
	ly := float32(a.winH)/2 - h/2
	if a.winW <= 0 {
		lx = 300
	}
	if a.winH <= 0 {
		ly = 300
	}
	menu := ui.Box(c).Absolute().Left(lx).Top(ly).Width(w).Height(h).Radius(12).Background(clrCard).PaddingY(4)
	menu.Draw(func(p *ui.Painter, r ui.Rect) {
		p.Shadow(r, 16, 0, 10, 36, 0, ui.RGBA(0, 0, 0, 0.45))
		p.FillRadii(r, [4]float32{12, 12, 12, 12}, clrCard)
	})
	menu.Children(func() {
		ui.Column(c).Gap(0).Children(func() {
			for _, it := range items {
				it := it
				row := ui.Box(c).Fill().Height(38).PaddingX(12)
				if row.Hovered() {
					row.Background(clrField)
				}
				row.Children(func() {
					ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Children(func() {
						ui.Text(c, it.label).FontSize(13).TextColor(clrText)
					})
				})
				if row.Clicked() {
					it.fn()
					a.win.Invalidate()
				}
			}
		})
	})
}

// playNext 下一首播放（对齐主线 play-next：插到当前歌后面 → 这里等价"立即播完这首接它"，
// MyGo 用 SetQueue 语义：取当前队列，把 item 插到 playIndex+1，重新 SetQueue）
func (a *app) playNext(item bilibili.SearchItem) {
	snap := a.pb.Snapshot()
	idx := snap.QIdx
	if idx < 0 || idx+1 > len(snap.Queue) {
		// 无队列：直接播放
		go a.play(item)
		return
	}
	q := make([]playback.Song, 0, len(snap.Queue)+1)
	q = append(q, snap.Queue[:idx+1]...)
	q = append(q, toSong(item))
	q = append(q, snap.Queue[idx+1:]...)
	a.pb.SetQueue(q, idx)
	a.win.Invalidate()
}

// copyLink 复制 B站链接到剪贴板（mygo Clipboard.WriteText）。
func (a *app) copyLink(item bilibili.SearchItem) {
	url := "https://www.bilibili.com/video/" + item.BVID
	mygo.Clipboard.WriteText(url)
	a.status = "已复制：" + url
	a.win.Invalidate()
}
