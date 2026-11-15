//go:build native

// 播放控制面板 + 队列/收藏/设置页 + 播放器控制方法（seek/volume/loop/next/prev/fav）。
package main

import (
	"fmt"
	"math/rand"

	"github.com/egoist/mygo/ui"

	"mymusic-mygo/bilibili"
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
	if a.loopMode == 2 { // 随机
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
	a.win.Invalidate()
}

// toggleLoop 循环模式：列表循环→单曲→随机（同步 SSOT）。
func (a *app) toggleLoop() {
	a.loopMode = (a.loopMode + 1) % 3
	a.pb.SetMode(intToMode(a.loopMode))
}

// loopLabel 当前循环模式名。
func (a *app) loopLabel() string {
	switch a.loopMode {
	case 1:
		return "单曲循环"
	case 2:
		return "随机播放"
	default:
		return "列表循环"
	}
}

// isFav 当前曲目是否已收藏。
func (a *app) isFav() bool {
	return a.current != nil && a.favMap[a.current.bvid]
}

// toggleFav 收藏/取消当前曲目。
func (a *app) toggleFav() {
	if a.current == nil {
		return
	}
	bvid := a.current.bvid
	a.win.Update(func() {
		if a.favMap[bvid] {
			delete(a.favMap, bvid)
			for i, f := range a.favorites {
				if f.BVID == bvid {
					a.favorites = append(a.favorites[:i], a.favorites[i+1:]...)
					break
				}
			}
		} else {
			a.favMap[bvid] = true
			a.favorites = append([]bilibili.SearchItem{{BVID: bvid, Title: a.current.title, Author: a.current.author, Pic: a.current.pic}}, a.favorites...)
		}
		a.win.Invalidate()
	})
}

// isFavItem 指定曲目是否已收藏。
func (a *app) isFavItem(item bilibili.SearchItem) bool {
	return a.favMap[item.BVID]
}

// toggleFavItem 收藏/取消指定曲目。
func (a *app) toggleFavItem(item bilibili.SearchItem) {
	bvid := item.BVID
	a.win.Update(func() {
		if a.favMap[bvid] {
			delete(a.favMap, bvid)
			for i, f := range a.favorites {
				if f.BVID == bvid {
					a.favorites = append(a.favorites[:i], a.favorites[i+1:]...)
					break
				}
			}
		} else {
			a.favMap[bvid] = true
			a.favorites = append([]bilibili.SearchItem{{BVID: bvid, Title: bilibili.CleanHTML(item.Title), Author: item.Author, Pic: item.Pic}}, a.favorites...)
		}
		a.win.Invalidate()
	})
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
			a.setVolume(float64(x / r.W))
		}
	})
	return box
}

// nowPlaying 正在播放覆盖面板：大封面 + 信息 + 大进度条(可 seek) + 控制 + 音量 + 歌词。
// 点击背景关闭。
func (a *app) nowPlaying(c *ui.Context) {
	ui.Overlay(c, func() {
		// 背景占满（Overlay 里必须 Absolute 定位）
		bg := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0)
		bg.Draw(func(p *ui.Painter, r ui.Rect) {
			// 深色背景 + 顶部淡光晕
			p.Fill(r, ui.RGBA(0x0e, 0x0f, 0x14, 0.97), 0)
			p.FillGradient(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: 170},
				ui.LinearGradient{From: ui.RGBA(0x8b, 0x5c, 0xf6, 0.16), To: ui.RGBA(0xec, 0x48, 0x99, 0.0), Angle: 270}, 170)
		})
		if bg.Clicked() {
			a.showPlayer = false
		}

		// 关闭按钮（右上）
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

		// 主体：上（封面+信息）下（歌词），Absolute 定位在中间区域
		ui.Column(c).Absolute().Left(a.resp(56, 24)).Top(20).Right(a.resp(56, 24)).Bottom(20).Gap(a.resp(28, 16)).Children(func() {
			// 上：左封面 + 右信息/控制（窄档<760 上下堆叠：封面居中上，信息下，对齐主线手机单栏）
			if a.narrow() {
				// 窄档结构：封面居中 + 信息/控制堆叠（对齐主线手机单栏）
				a.loadCover(a.current.bvid, a.current.pic)
				a.narrowInfo(c)
			} else {
				ui.Row(c).Gap(a.resp(52, 24)).AlignItems(ui.Center).Children(func() {
					// 左：大封面 + 光晕
					cover := ui.Box(c).Size(a.resp(250, 170), a.resp(250, 170)).Radius(a.resp(24, 18))
					cover.Draw(func(p *ui.Painter, r ui.Rect) {
						p.Shadow(r, 24, 0, 8, 40, 0, ui.RGBA(0x8b, 0x5c, 0xf6, 0.35))
						p.FillRadii(r, [4]float32{24, 24, 24, 24}, clrCard)
						p.Clip(r, 24, func() {
							if img, ok := a.covers[a.current.bvid]; ok {
								p.Image(img, r, ui.Cover)
							} else {
								p.FillGradient(r, grad(), 24)
							}
						})
					})
					a.loadCover(a.current.bvid, a.current.pic)

					// 右：信息 + 进度 + 控制 + 音量
					ui.Column(c).Grow(1).Gap(14).Children(func() {
						ui.Text(c, a.current.title).FontSize(a.resp(26, 20)).Bold().TextColor(clrText).MaxLines(2)
						ui.Text(c, a.current.author).FontSize(a.resp(15, 13)).TextColor(clrMuted)

						// 大进度条（可 seek）
						ui.Spacer(c).Height(10)
						a.progressBarSized(c, a.progress, 8, true).Grow(1)
						ui.Row(c).AlignSelf(ui.Stretch).Children(func() {
							ui.Text(c, a.posText).FontSize(13).TextColor(clrMuted)
							ui.Spacer(c).Grow(1)
							ui.Text(c, a.durText).FontSize(13).TextColor(clrMuted)
						})

						// 控制行：循环 上一首 播放 下一首 收藏
						ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(a.resp(18, 12)).Children(func() {
							ui.Spacer(c).Grow(1)
							if iconButton2(c, icRepeat, a.resp(40, 34), accent(a.loopMode > 0)).Clicked() {
								a.toggleLoop()
							}
							if iconButton2(c, icPrev, a.resp(44, 38), clrMuted).Clicked() {
								go a.prev()
							}
							if playCircle(c, a.playing, a.resp(72, 58)).Clicked() {
								a.togglePlay()
							}
							if iconButton2(c, icNext, a.resp(44, 38), clrMuted).Clicked() {
								go a.next()
							}
							if iconButton2(c, icHeart, a.resp(40, 34), accent(a.isFav())).Clicked() {
								a.toggleFav()
							}
							ui.Spacer(c).Grow(1)
						})

						// 音量 + 音质
						ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(12).Children(func() {
							icon(c, icVol, 18, clrMuted)
							a.volumeBar(c, a.resp(200, 130))
							ui.Spacer(c).Grow(1)
							ui.Text(c, "192k").FontSize(13).TextColor(clrMuted)
							ui.Text(c, a.loopLabel()).FontSize(13).TextColor(clrMuted)
						})
					})
				})
			}

			// 下：歌词
			a.lyricStrip(c)
		})
	})
}

// narrowInfo 窄档（<760）的播放面板：封面居中 + 下方信息/控制堆叠。
func (a *app) narrowInfo(c *ui.Context) {
	ui.Row(c).AlignSelf(ui.Center).Children(func() {
		cn := ui.Box(c).Size(150, 150).Radius(18)
		cn.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Shadow(r, 24, 0, 8, 40, 0, ui.RGBA(0x8b, 0x5c, 0xf6, 0.35))
			p.FillRadii(r, [4]float32{24, 24, 24, 24}, clrCard)
			if img, ok := a.covers[a.current.bvid]; ok {
				p.Clip(r, 18, func() { p.Image(img, r, ui.Cover) })
			} else {
				p.Clip(r, 18, func() { p.FillGradient(r, grad(), 18) })
			}
		})
	})
	ui.Column(c).AlignSelf(ui.Stretch).Gap(a.resp(14, 12)).Children(func() {
		ui.Column(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.current.title).FontSize(20).Bold().TextColor(clrText).MaxLines(2).AlignSelf(ui.Center)
			ui.Text(c, a.current.author).FontSize(13).TextColor(clrMuted)
		})
		a.progressBarSized(c, a.progress, 6, true).Grow(1)
		ui.Row(c).AlignSelf(ui.Stretch).Children(func() {
			ui.Text(c, a.posText).FontSize(12).TextColor(clrMuted)
			ui.Spacer(c).Grow(1)
			ui.Text(c, a.durText).FontSize(12).TextColor(clrMuted)
		})
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(14).Children(func() {
			ui.Spacer(c).Grow(1)
			if iconButton2(c, icRepeat, 32, accent(a.loopMode > 0)).Clicked() {
				a.toggleLoop()
			}
			if iconButton2(c, icPrev, 36, clrMuted).Clicked() {
				go a.prev()
			}
			if playCircle(c, a.playing, 54).Clicked() {
				a.togglePlay()
			}
			if iconButton2(c, icNext, 36, clrMuted).Clicked() {
				go a.next()
			}
			if iconButton2(c, icHeart, 32, accent(a.isFav())).Clicked() {
				a.toggleFav()
			}
			ui.Spacer(c).Grow(1)
		})
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10).Children(func() {
			icon(c, icVol, 16, clrMuted)
			a.volumeBar(c, 110)
			ui.Spacer(c).Grow(1)
			ui.Text(c, "192k").FontSize(12).TextColor(clrMuted)
		})
	})
}

// lyricStrip 歌词区（占位模拟；真实 B站歌词后续接入）。
func (a *app) lyricStrip(c *ui.Context) {
	ui.Column(c).Gap(10).AlignItems(ui.Center).Children(func() {
		if a.current == nil {
			return
		}
		ui.Text(c, "歌词").FontSize(11).TextColor(clrMuted)
		lines := []string{"你发如雪 凄美了离别", "我焚香感动了谁", "邀明月 让回忆皎洁", "爱在月光下完美"}
		cur := 2 // 模拟当前行
		for i, ln := range lines {
			col := clrMuted
			size := float32(15)
			if i == cur {
				col = ui.RGB(0xff, 0xff, 0xff)
				size = 20
			}
			ui.Text(c, ln).FontSize(size).Bold().TextColor(col)
		}
	})
}

// queuePage 队列页：上次在听 + 播放列表。
func (a *app) queuePage(c *ui.Context) {
	ui.Column(c).Fill().PaddingX(a.resp(24, 12)).PaddingY(a.resp(16, 10)).Gap(14).Children(func() {
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
			ui.Column(c).Fill().Center().Children(func() {
				ui.Text(c, "队列为空，去搜索添加").TextColor(clrMuted).FontSize(15)
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
					a.win.Update(func() {
						a.favorites = nil
						a.favMap = map[string]bool{}
						a.win.Invalidate()
					})
				}
			}
		})
		if len(a.favorites) == 0 {
			ui.Column(c).Fill().Center().Children(func() {
				ui.Text(c, "还没有收藏，播放时点 ♥ 收藏").TextColor(clrMuted).FontSize(15)
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
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			a.cover(c, item.BVID, item.Pic, a.resp(44, 36))
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Text(c, item.Title).FontSize(a.resp(14, 13)).Bold().TextColor(clrText).MaxLines(1)
				ui.Text(c, item.Author).FontSize(12).TextColor(clrMuted).MaxLines(1)
			})
			if iconButton2(c, icHeart, 32, clrPink).Clicked() {
				bvid := item.BVID
				a.win.Update(func() {
					delete(a.favMap, bvid)
					for j, f := range a.favorites {
						if f.BVID == bvid {
							a.favorites = append(a.favorites[:j], a.favorites[j+1:]...)
							break
						}
					}
				})
			}
		})
	})
	if box.Clicked() {
		go a.play(item)
	}
}

// settingsPage 设置页：简单设置项。
func (a *app) settingsPage(c *ui.Context) {
	ui.Column(c).Fill().PaddingX(a.resp(24, 12)).PaddingY(a.resp(16, 10)).Gap(12).Children(func() {
		ui.Text(c, "设置").FontSize(20).Bold().TextColor(clrText)
		ui.Spacer(c).Height(6)
		a.settingRow(c, "音质", "192k（B站最高音质）", "")
		a.settingRow(c, "循环模式", a.loopLabel(), "点击切换")
		a.settingRow(c, "桌面歌词挂件", "", "播放条右上角切换")
		a.settingRow(c, "缓存目录", "/tmp/mymusic-*.m4a", "临时文件")
		ui.Spacer(c).Grow(1)
		ui.Text(c, "MyGo 原生版 · 紫粉渐变主题").FontSize(12).TextColor(clrMuted)
	})
}

// settingRow 设置项行。
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

// castPage 投屏占位页。
func (a *app) castPage(c *ui.Context) {
	ui.Column(c).Fill().Center().Gap(12).Children(func() {
		icon(c, icCast, 44, clrMuted)
		ui.Text(c, "投屏功能开发中").FontSize(17).TextColor(clrMuted)
		ui.Text(c, "支持 DLNA / AirPlay 投送到电视、音箱").FontSize(13).TextColor(clrMuted)
	})
}
