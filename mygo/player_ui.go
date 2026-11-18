//go:build native

// 播放控制面板 + 队列/收藏/设置页 + 播放器控制方法（seek/volume/loop/next/prev/fav）。
package main

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"homecast/service/lyric"
	"homecast/service/speaker"

	"homecast/service/fav"

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

// next 下一首。对齐主线 d-next / cmd('next')：后端 Next 纯环绕顺序
// （(QIdx+1)%len），random 只是 UI 图标（主线后端 Next 不随机，前端也直接 cmd('next')），
// 不自创随机跳转。
func (a *app) next() {
	a.pb.Next()
}

// prev 上一首。对齐主线 d-prev / cmd('prev')：后端 Prev 纯环绕顺序
// （(QIdx-1+len)%len），无 ">3s 回到开头" 自创逻辑。
func (a *app) prev() {
	a.pb.Prev()
}

// seek 按比例跳转进度（走 SSOT）。
// 音箱模式：playback.Seek → SpeakerOutput.Seek → speaker.SeekTo → 投带 start=N 的新 URL
// （ffmpeg 切剩余段，代价 1~2s）。本地模式：MyGoOutput.Seek 直接在解码器定位。
func (a *app) seek(frac float64) {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	dur := a.pb.Snapshot().Duration
	if dur <= 0 {
		return // 时长未知不 seek（否则 seek 到 0 变成从头播）
	}
	a.pb.Seek(frac * dur)
}

// setVolume 设置音量 0..1。音箱模式（source=speaker）走主线 playback.SetVolume
// （out=SpeakerOutput 真调音箱 Control，复用主线不造轮子）；web 模式走 MyGo 本地 audio。
func (a *app) setVolume(v float64) {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	a.volume = v
	if a.spkMode {
		// 音箱模式：复用主线（SpeakerOutput.SetVolume → speaker.Control volume）
		a.pb.SetVolume(int(v * 100))
	} else if a.mygoOut != nil {
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
	// 对齐主线 d-vol：<input type=range> 原生可拖。改用 mygo 原生 SliderBase
	// （flagDraggable，绑定 &a.volume 双向）：之前自定义 Box+PointerPosition 拖不动。
	// Draw 自定义轨道/填充/thumb（高 6 粉渐变 + 12px 圆 thumb，thumb 始终显示）。
	const h = float32(6)
	pad := float32(10) // 半 thumb 宽 padding，让 thumb 不出界
	s := ui.SliderBase(c, &a.volume, 0, 1).Width(width).Height(h + 2*pad).PaddingX(pad)
	s.Draw(func(p *ui.Painter, r ui.Rect) {
		track := ui.Rect{X: r.X, Y: r.Y + (r.H-h)/2, W: r.W, H: h}
		p.FillRadii(track, [4]float32{3, 3, 3, 3}, clrTrackBar)
		v := a.volume
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		cy := track.Y + h/2
		if v > 0 {
			fr := ui.Rect{X: track.X, Y: track.Y, W: track.W * float32(v), H: h}
			p.FillGradient(fr, grad(), 3)
		}
		// 圆形 thumb 始终显示（volume 0 也在最左，对齐主线 range thumb 常驻）
		cx := track.X + track.W*float32(v)
		th := ui.Rect{X: cx - 6, Y: cy - 6, W: 12, H: 12}
		p.FillRadii(th, [4]float32{6, 6, 6, 6}, ui.RGB(0xff, 0xff, 0xff))
		p.FillRadii(ui.Rect{X: th.X + 1, Y: th.Y + 1, W: th.W - 2, H: th.H - 2}, [4]float32{5, 5, 5, 5}, clrPink)
		// hover 百分比气泡
		if s.Hovered() {
			pct := int(a.volume * 100)
			if a.muted {
				pct = 0
			}
			// 纯数字百分比，不带外框（用户：外框气泡有点崩，纯数字即可）
			label := fmt.Sprintf("%d%%", pct)
			p.Text(r.X+r.W/2-6, r.Y-14, label, 11, clrText)
		}
	})
	if s.Changed() {
		a.setVolume(a.volume) // 拖动改 a.volume 后应用音频音量
	}
	return s
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
		// 点背景仅收起窄档抽屉（对齐主线 closeDrawers；面板不随背景点击关闭，
		// 关闭靠 ly-close / ESC / 封面再点）
		if bg.Clicked() {
			a.lyrDrawer = 0
		}
		// ESC 关闭歌词面板（对齐主线 lyric.js ESC）
		if c.Shortcut(0, ui.KeyEscape) {
			a.showPlayer = false
			a.lyrDrawer = 0
		}
		// 关闭按钮（对齐主线 ly-close：右上角）
		ui.Row(c).Absolute().Top(18).Right(20).Children(func() {
			if iconButton(c, "✕").Clicked() {
				a.showPlayer = false
				a.lyrDrawer = 0
			}
		})

		if a.current == nil {
			ui.Column(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).Center().Children(func() {
				ui.Text(c, "未在播放").FontSize(22 * uiScale).Bold().TextColor(clrMuted)
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
	box := ui.Box(c).Width(a.resp(256, 200)).Radius(16 * uiScale).Background(clrCardGlass).PaddingX(10 * uiScale).PaddingY(8 * uiScale)
	box.Children(func() {
		ui.Column(c).Gap(6 * uiScale).Children(func() {
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "歌词候选").FontSize(13 * uiScale).Bold().TextColor(clrPink)
				ui.Spacer(c).Grow(1)
				// 候选标题《歌名》（对齐主线 ly-cand-title）
				ui.Text(c, "《"+titleToSongMygo(bilibili.CleanHTML(a.current.title))+"》").
					FontSize(11).TextColor(clrText).MaxLines(1).MaxWidth(120)
			})
			if len(a.lyricCands) == 0 {
				ui.Text(c, LaodingTag()).FontSize(12 * uiScale).TextColor(clrMuted).PaddingY(20 * uiScale)
			} else {
				ui.Scroll(c).Grow(1).Gap(4 * uiScale).Scrollbars(true).Children(func() {
					ui.Column(c).Gap(4 * uiScale).Children(func() {
						for _, cand := range a.lyricCands {
							cand := cand
							on := a.lyricSID == cand.ID
							row := ui.Box(c).Fill().Height(52 * uiScale).Radius(10 * uiScale).PaddingX(10 * uiScale)
							if on {
								// 选中态：粉紫底（对齐主线 bg-pink-500/20 text-pink-300）
								row.Background(ui.RGBA(0xec, 0x48, 0x99, 0.20))
							} else if row.Hovered() {
								row.Background(ui.RGBA(0x2b, 0x2c, 0x37, 0.6))
							}
							row.Children(func() {
								// 左对齐两行：歌名 / 歌手·时长（对齐主线 text-left）
								ui.Column(c).Grow(1).Gap(2 * uiScale).AlignSelf(ui.Center).Children(func() {
									ui.Text(c, cand.Name).FontSize(13 * uiScale).Bold().
										TextColor(selColor(on, clrPink, clrText)).MaxLines(1)
									ui.Text(c, cand.Artist+" · "+candDurText(cand.Duration)).
										FontSize(11).TextColor(clrMuted).MaxLines(1)
								})
							})
							if row.Clicked() {
								go a.pickLyricSID(a.current.bvid, cand.ID)
							}
						}
					})
				})
			}

		})
	})
}

// LaodingTag 加载中文态。
func LaodingTag() string { return "搜索中…" }

// lyrCenterCol 中栏：歌曲信息 + KTV 歌词列表 + 底部进度条（对齐主线 lyric-panel 中栏）。
func (a *app) lyrCenterCol(c *ui.Context) {
	ui.Column(c).Grow(1).Fill().Children(func() {
		// 顶部：封面 + 标题/歌手/来源（对齐主线 ly-cover/ly-title/ly-artist/ly-source）
		// 上边距 resp 24/14（对齐主线 mb-3 sm:mb-6）
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(12).PaddingX(4).Margin(0, 0, a.resp(24, 14), 0).Children(func() {
			a.cover(c, a.current.bvid, a.current.pic, a.resp(64, 48))
			ui.Column(c).Grow(1).Gap(3 * uiScale).Children(func() {
				// 标题 text-lg(18px) 白 / 歌手 text-sm(14px) gray / 来源 text-xs(12px) gray（对齐主线）
				ui.Text(c, bilibili.CleanHTML(a.current.title)).FontSize(a.resp(18, 15)).Bold().TextColor(clrText).MaxLines(1)
				ui.Text(c, a.current.author).FontSize(14 * uiScale).TextColor(clrMuted).MaxLines(1)
				// 来源行（对齐主线 ly-source：网易云 候选名《歌名》/ 未找到引导）
				ui.Text(c, a.lyricSourceText()).FontSize(12 * uiScale).TextColor(clrPink).MaxLines(1)
			})
		})
		// KTV 歌词列表（对齐主线 ly-lines：居中 + 当前行粉色渐底逐行进度 + 点击行跳句）
		// 优化：垂直渐变玻璃（顶 0.78 → 底 0.92）更立体，歌词在玻璃上
		lyrCard := ui.Box(c).Grow(1).Fill().Radius(14 * uiScale).PaddingX(a.resp(28, 14)).PaddingY(10 * uiScale)
		lyrCard.Draw(func(p *ui.Painter, r ui.Rect) {
			p.FillGradient(r, ui.LinearGradient{From: ui.RGBA(0x18, 0x19, 0x22, 0.78), To: ui.RGBA(0x0e, 0x0f, 0x14, 0.92), Angle: 90}, 14)
		})
		lyrCard.Children(func() {
			curIdx, prog := a.lyricProg(a.posSec)
			ui.Scroll(c).Fill().Gap(10 * uiScale).Scrollbars(true).Children(func() {
				ui.Column(c).AlignSelf(ui.Stretch).Gap(8 * uiScale).Children(func() {
					if len(a.lyricLines) == 0 {
						ui.Column(c).AlignSelf(ui.Stretch).Children(func() {
							ui.Text(c, "歌词加载中…").FontSize(15 * uiScale).TextColor(clrMuted).AlignSelf(ui.Center).PaddingY(40 * uiScale)
							ui.Text(c, "网易云词源（搜索："+lyric.CleanSearchKeyword(bilibili.CleanHTML(a.current.title))+"）").FontSize(11 * uiScale).TextColor(clrMuted).AlignSelf(ui.Center)
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
						active := i == curIdx
						if active {
							col = ui.RGB(0xff, 0xff, 0xff)
							size = a.lyricFontSize() * 1.2
						}
						// 行容器：居中 + 点击跳句（对齐主线 ly-line cursor-pointer text-center）
						row := ui.Box(c).AlignSelf(ui.Stretch).PaddingY(5 * uiScale).PaddingX(16 * uiScale).Radius(8 * uiScale)
						row.Draw(func(p *ui.Painter, r ui.Rect) {
							// 卡拉OK渐进底（对齐主线 .ly-line.ly-active --prog 粉色渐底）
							// 优化：当前行渐底更饱和 + 左右粉/紫装饰竖条（更醒目）
							if active {
								p.FillGradient(ui.Rect{X: r.X, Y: r.Y, W: r.W * float32(prog), H: r.H},
									ui.LinearGradient{From: ui.RGBA(0xec, 0x48, 0x99, 0.44), To: ui.RGBA(0x8b, 0x5c, 0xf6, 0.18), Angle: 0}, 8)
								// 左粉 / 右紫装饰竖条（仅当前行）
								p.Fill(ui.Rect{X: r.X + 4, Y: r.Y + r.H*0.22, W: 3, H: r.H * 0.56}, clrPink, 2)
								p.Fill(ui.Rect{X: r.X + r.W - 7, Y: r.Y + r.H*0.22, W: 3, H: r.H * 0.56}, clrPurple, 2)
							}
							if row.Hovered() && !active {
								p.Fill(r, ui.RGBA(0xff, 0xff, 0xff, 0.05), 8)
							}
						})
						row.Children(func() {
							// 时间戳+歌词整体居中（对齐主线 .ly-line justify-center gap-2）
							ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Justify(ui.Center).Gap(8 * uiScale).Children(func() {
								if a.lyricTime {
									// 每行都有时间戳（对齐 renderLines lyTime：text-[10px] gray 小字）
									ui.Text(c, lyricTimeStr(lyricLineTime(ln))).FontSize(10 * uiScale).TextColor(clrMuted)
								}
								ui.Text(c, text).FontSize(size).Bold().TextColor(col).LineHeight(1.8)
							})
						})
						// KTV 跟随：切到新行时把当前行滚到中栏中心
						if active && curIdx != a.lastLyricIdx {
							row.ScrollIntoView(ui.ScrollIntoViewOptions{Block: ui.ScrollCenter})
						}
						// 点击行 → 跳该句时间（对齐主线 renderLines click）
						if row.Clicked() {
							a.seekToLine(i)
						}
					}
					a.lastLyricIdx = curIdx
				})
			})
		})
		// 底部进度条（对齐主线 ly-progress：可拖 + 当前/总时长；上边距 mt-2 sm:mt-4）
		ui.Row(c).AlignSelf(ui.Stretch).Gap(10).AlignItems(ui.Center).Margin(a.resp(16, 10), 0, 0, 0).Children(func() {
			ui.Text(c, a.posText).FontSize(12 * uiScale).TextColor(clrMuted)
			a.progressBarSized(c, a.progress, 6, true).Grow(1)
			ui.Text(c, a.durText).FontSize(12 * uiScale).TextColor(clrMuted)
		})
	})
}

// lyrToolCol 右栏：工具列（对齐主线 ly-tool-drawer：字号± / 对齐±0.5s / 刷新）。
func (a *app) lyrToolCol(c *ui.Context) {
	ui.Column(c).Width(56 * uiScale).Gap(8 * uiScale).Children(func() {
		toolCard := func(fn func()) {
			box := ui.Box(c).Fill().Radius(14 * uiScale).Background(clrCardGlass).PaddingX(6 * uiScale).PaddingY(10 * uiScale)
			box.Children(fn)
		}
		// 竖排播放控制卡（用户要求控制面竖排，放右侧工具列）
		toolCard(func() {
			ui.Column(c).AlignSelf(ui.Center).Gap(8 * uiScale).Children(func() {
				if iconButton2(c, icPrev, 32, clrMuted).Clicked() {
					go a.prev()
				}
				if playCircle(c, a.playing, 42).Clicked() {
					a.togglePlay()
				}
				if iconButton2(c, icNext, 32, clrMuted).Clicked() {
					go a.next()
				}
			})
		})
		toolCard(func() {
			ui.Column(c).AlignSelf(ui.Center).Gap(6 * uiScale).Children(func() {
				ui.Text(c, "字号").FontSize(11 * uiScale).TextColor(clrMuted)
				if iconButton(c, "＋").Clicked() {
					if a.lyricSize < 4 {
						a.lyricSize++
						a.savePref("mygo_lyricsize", a.lyricSize)
						a.win.Invalidate()
					}
				}
				// 档位显示（对齐主线 ly-size 3/5）
				ui.Text(c, fmt.Sprintf("%d/5", a.lyricSize+1)).FontSize(11 * uiScale).TextColor(clrText)
				if iconButton(c, "－").Clicked() {
					if a.lyricSize > 0 {
						a.lyricSize--
						a.savePref("mygo_lyricsize", a.lyricSize)
						a.win.Invalidate()
					}
				}
			})
		})
		toolCard(func() {
			ui.Column(c).AlignSelf(ui.Center).Gap(6 * uiScale).Children(func() {
				ui.Text(c, "对齐").FontSize(11 * uiScale).TextColor(clrMuted)
				if iconButton(c, "▲").Clicked() {
					a.nudgeLyric(0.5)
				}
				// 偏移值：点击归零（对齐主线 ly-nudge-val 点击归零）
				off := ui.Box(c).PaddingX(2 * uiScale).Radius(4 * uiScale)
				if off.Hovered() {
					off.Background(clrField)
				}
				off.Children(func() {
					ui.Text(c, fmt.Sprintf("%.1fs", a.lyricOffset)).FontSize(10 * uiScale).TextColor(clrPink)
				})
				if off.Clicked() && a.lyricOffset != 0 {
					a.setLyricOffset(0)
				}
				if iconButton(c, "▼").Clicked() {
					a.nudgeLyric(-0.5)
				}
			})
		})
		toolCard(func() {
			ui.Column(c).AlignSelf(ui.Center).Gap(6 * uiScale).Children(func() {
				// 收藏/取消收藏（对齐主线 ly-fav）
				if iconButton2(c, icHeart, 26, accent(a.isFav())).Clicked() {
					a.toggleFav()
				}
				// 复制链接（对齐主线 ly-copy）
				if iconButton(c, "⧉").Clicked() {
					it := *a.currentToItem()
					a.copyLink(it)
				}
			})
		})
	})
}

// narrowLyricPanel 窄档歌词面板：全宽 KTV 歌词 + 候选/工具抽屉
// （对齐主线 <640：左上 ly-drawer-cand / ly-drawer-tool 按需唤出，互斥、点外收起）。
func (a *app) narrowLyricPanel(c *ui.Context) {
	ui.Row(c).Fill().Gap(a.resp(12, 8)).Children(func() {
		ui.Column(c).Grow(1).Fill().Children(func() {
			// 顶栏：抽屉按钮 + 封面/标题
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Children(func() {
				if iconButton2(c, icMenu, 32, accent(a.lyrDrawer == 1)).Clicked() {
					if a.lyrDrawer == 1 {
						a.lyrDrawer = 0
					} else {
						a.lyrDrawer = 1
					}
					a.win.Invalidate()
				}
				a.cover(c, a.current.bvid, a.current.pic, 42)
				ui.Column(c).Grow(1).Gap(2 * uiScale).Children(func() {
					ui.Text(c, bilibili.CleanHTML(a.current.title)).FontSize(15 * uiScale).Bold().TextColor(clrText).MaxLines(1)
					ui.Text(c, a.current.author).FontSize(11 * uiScale).TextColor(clrMuted).MaxLines(1)
				})
				if iconButton2(c, icLyric, 32, accent(a.lyricShown)).Clicked() {
					a.toggleLyric()
				}
				// 工具抽屉按钮（对齐主线 ly-drawer-tool）
				if iconButton2(c, icSet, 32, accent(a.lyrDrawer == 2)).Clicked() {
					if a.lyrDrawer == 2 {
						a.lyrDrawer = 0
					} else {
						a.lyrDrawer = 2
					}
					a.win.Invalidate()
				}
			})
			// 抽屉浮层（候选 / 工具，互斥）
			if a.lyrDrawer == 1 {
				ui.Box(c).AlignSelf(ui.Stretch).Radius(12 * uiScale).Background(clrCardGlass).PaddingX(10 * uiScale).PaddingY(8 * uiScale).Children(func() {
					ui.Column(c).Gap(6 * uiScale).Children(func() {
						ui.Text(c, "歌词候选").FontSize(13 * uiScale).Bold().TextColor(clrPink)
						if len(a.lyricCands) == 0 {
							ui.Text(c, LaodingTag()).FontSize(12 * uiScale).TextColor(clrMuted)
							return
						}
						ui.Scroll(c).MaxHeight(200).Gap(4 * uiScale).Scrollbars(true).Children(func() {
							ui.Column(c).Gap(4 * uiScale).Children(func() {
								for _, cand := range a.lyricCands {
									cand := cand
									on := a.lyricSID == cand.ID
									row := ui.Box(c).AlignSelf(ui.Stretch).Height(46 * uiScale).Radius(8 * uiScale).PaddingX(10 * uiScale)
									if on {
										row.Background(ui.RGBA(0xec, 0x48, 0x99, 0.20))
									} else if row.Hovered() {
										row.Background(ui.RGBA(0x2b, 0x2c, 0x37, 0.6))
									}
									row.Children(func() {
										ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Children(func() {
											ui.Column(c).Grow(1).Gap(2 * uiScale).Children(func() {
												ui.Text(c, cand.Name).FontSize(13 * uiScale).Bold().
													TextColor(selColor(on, clrPink, clrText)).MaxLines(1)
												ui.Text(c, cand.Artist+" · "+candDurText(cand.Duration)).FontSize(11 * uiScale).TextColor(clrMuted).MaxLines(1)
											})
										})
									})
									if row.Clicked() {
										go a.pickLyricSID(a.current.bvid, cand.ID)
										a.lyrDrawer = 0
									}
								}
							})
						})
					})
				})
			} else if a.lyrDrawer == 2 {
				ui.Box(c).AlignSelf(ui.Stretch).Radius(12 * uiScale).Background(clrCardGlass).PaddingX(12 * uiScale).PaddingY(10 * uiScale).Children(func() {
					ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Wrap().Children(func() {
						ui.Text(c, "字号").FontSize(11 * uiScale).TextColor(clrMuted)
						if iconButton(c, "＋").Clicked() {
							if a.lyricSize < 4 {
								a.lyricSize++
								a.savePref("mygo_lyricsize", a.lyricSize)
								a.win.Invalidate()
							}
						}
						ui.Text(c, fmt.Sprintf("%d/5", a.lyricSize+1)).FontSize(11 * uiScale).TextColor(clrText)
						if iconButton(c, "－").Clicked() {
							if a.lyricSize > 0 {
								a.lyricSize--
								a.savePref("mygo_lyricsize", a.lyricSize)
								a.win.Invalidate()
							}
						}
						ui.Spacer(c).Width(8 * uiScale)
						ui.Text(c, "对齐").FontSize(11 * uiScale).TextColor(clrMuted)
						if iconButton(c, "▲").Clicked() {
							a.nudgeLyric(0.5)
						}
						ui.Text(c, fmt.Sprintf("%.1fs", a.lyricOffset)).FontSize(10 * uiScale).TextColor(clrPink)
						if iconButton(c, "▼").Clicked() {
							a.nudgeLyric(-0.5)
						}
						if iconButton2(c, icHeart, 26, accent(a.isFav())).Clicked() {
							a.toggleFav()
						}
					})
				})
			}
			// KTV 歌词（居中 + 渐底 + 点击跳句，同中栏）
			ui.Box(c).Grow(1).Fill().Radius(12 * uiScale).Background(clrCardGlass).PaddingX(12 * uiScale).PaddingY(10 * uiScale).Children(func() {
				curIdx, prog := a.lyricProg(a.posSec)
				ui.Scroll(c).Fill().Gap(8 * uiScale).Scrollbars(true).Children(func() {
					ui.Column(c).AlignSelf(ui.Stretch).Gap(8 * uiScale).Children(func() {
						if len(a.lyricLines) == 0 {
							ui.Text(c, "歌词加载中…").FontSize(14 * uiScale).TextColor(clrMuted).AlignSelf(ui.Center).PaddingY(30 * uiScale)
							return
						}
						for i, ln := range a.lyricLines {
							i, ln := i, ln
							text := lyricLineText(ln)
							if text == "" {
								continue
							}
							active := i == curIdx
							col := clrMuted
							size := a.lyricFontSize()
							if active {
								col = ui.RGB(0xff, 0xff, 0xff)
								size = a.lyricFontSize() * 1.3
							}
							row := ui.Box(c).AlignSelf(ui.Stretch).PaddingY(4 * uiScale).PaddingX(8 * uiScale).Radius(8 * uiScale)
							row.Draw(func(p *ui.Painter, r ui.Rect) {
								if active {
									p.FillGradient(ui.Rect{X: r.X, Y: r.Y, W: r.W * float32(prog), H: r.H},
										ui.LinearGradient{From: ui.RGBA(0xec, 0x48, 0x99, 0.30), To: ui.RGBA(0xec, 0x48, 0x99, 0.14), Angle: 0}, 8)
								}
							})
							row.Children(func() {
								ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Justify(ui.Center).Gap(8 * uiScale).Children(func() {
									if a.lyricTime {
										ui.Text(c, lyricTimeStr(lyricLineTime(ln))).FontSize(10 * uiScale).TextColor(clrMuted)
									}
									ui.Text(c, text).FontSize(size).Bold().TextColor(col).LineHeight(1.8)
								})
							})
							if active && curIdx != a.lastLyricIdx {
								row.ScrollIntoView(ui.ScrollIntoViewOptions{Block: ui.ScrollCenter})
							}
							if row.Clicked() {
								a.seekToLine(i)
							}
						}
						a.lastLyricIdx = curIdx
					})
				})
			})
			ui.Row(c).AlignSelf(ui.Stretch).Gap(8 * uiScale).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.posText).FontSize(12 * uiScale).TextColor(clrMuted)
				a.progressBarSized(c, a.progress, 5, true).Grow(1)
				ui.Text(c, a.durText).FontSize(12 * uiScale).TextColor(clrMuted)
			})
		})
	})
}

// selColor 选中态颜色选择（简化条件表达式，Go 无三元）。
func selColor(on bool, yes, no ui.Color) ui.Color {
	if on {
		return yes
	}
	return no
}

// queuePage 队列页：最近播放 + 上次在听 + 播放列表（对齐主线 last-view/recent-view/queue-view）。
func (a *app) queuePage(c *ui.Context) {
	// 整页滚动条：整页内容包一个 Scroll，不做组件内部滚动条（最近播放横排/队列列表不再单独 Scroll）
	ui.Scroll(c).Grow(1).Scrollbars(true).Children(func() {
		ui.Column(c).Fill().PaddingX(a.resp(24, 12)).PaddingY(a.resp(16, 10)).Gap(14 * uiScale).Children(func() {
			// 最近播放横排小卡（对齐主线 recent-view：最多 10 显示 + 清空）
			if len(a.recent) > 0 {
				ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Children(func() {
					ui.Text(c, "最近播放").FontSize(16 * uiScale).Bold().TextColor(clrText)
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
				ui.ScrollHorizontal(c).Height(rowH).Scrollbars(true).Gap(10 * uiScale).Children(func() {
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
func (a *app) recentPage(c *ui.Context) {
	// 整页滚动条（对齐主线）：整页内容包一个 Scroll，不做组件内部滚动条
	ui.Scroll(c).Grow(1).Scrollbars(true).Children(func() {
		ui.Column(c).Fill().PaddingX(a.resp(24, 12)).PaddingY(a.resp(16, 10)).Gap(14 * uiScale).Children(func() {
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
				ui.Text(c, "最近播放").FontSize(20 * uiScale).Bold().TextColor(clrText)
				ui.Text(c, fmt.Sprintf("共 %d 首", len(a.recent))).FontSize(13 * uiScale).TextColor(clrMuted)
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
				ui.Column(c).Fill().Center().Gap(10 * uiScale).Children(func() {
					icon(c, icList, 44, ui.RGBA(0xff, 0xff, 0xff, 0.08))
					ui.Text(c, "还没有播放记录").FontSize(17 * uiScale).Bold().TextColor(clrText)
					ui.Text(c, "听过的歌会出现在这里，最多保留 30 首").FontSize(13 * uiScale).TextColor(clrMuted)
					if gradButton(c, "去搜索").Clicked() {
						a.setTab("search")
					}
				})
				return
			}
			// 网格卡片（列数按 tier 对应主线 grid-cols-3 sm:4 md:6）+ 分页
			cols := a.recentGridCols()
			tracks := make([]ui.Track, cols)
			for i := range tracks {
				tracks[i] = ui.Fr(1)
			}
			page := a.pg["recent"]
			ui.Grid(c).ColumnTracks(tracks...).Gap(12 * uiScale).Fill().Children(func() {
				for _, item := range pageSlice(a.recent, page) {
					a.recentCard(c, item)
				}
			})
			pager(c, page, pageCount(len(a.recent)), func(p int) {
				a.pg["recent"] = p
				a.win.Invalidate()
			})
		})
	})
}

// recentGridCols 最近播放网格列数（对齐主线 grid-cols-3 sm:4 md:6）。
func (a *app) recentGridCols() int {
	switch a.tier() {
	case 0: // 手机（<768）
		return 3
	case 1: // 紧凑（768-1024）
		return 4
	default: // 桌面（>=1024）
		return 6
	}
}

// recentCard 最近播放网格卡片：方形封面为主 + 标题/歌手文字次之（对齐主线 recent.js 卡片）。
// gridCard 网格卡片共享样式（封面盒 + 12 圆角 Clip + hover 遮罩 + 标题/副标题）。
// recentCard / songlistCard 等网格卡片全部复用，避免重复造轮子。
// coverKey=封面缓存键（bvid 或歌单 key）；coverURL 空=不拉取；onTap=点击回调。
// gridCard 网格卡片（最近播放 / 歌单共用）：方形封面 + 标题 + 副标题。
// 整体包在 Column 里作为单一 grid item，避免封面与文字被排成不同格子。
func (a *app) gridCard(c *ui.Context, coverKey, coverURL, title, subtitle string, size float32, onTap func()) {
	ui.Column(c).Width(size).Gap(6 * uiScale).Children(func() {
		box := ui.Box(c).Width(size).Height(size).Radius(12 * uiScale)
		box.Draw(func(p *ui.Painter, b ui.Rect) {
			// 无图时用 clrTrack 占位（比 clrCard 浅，避免看起来全黑）
			if img, ok := a.covers[coverKey]; ok {
				p.Clip(b, 12, func() { p.Image(img, b, ui.Cover) })
			} else {
				p.FillRadii(b, [4]float32{12, 12, 12, 12}, clrTrack)
			}
		})
		if coverURL != "" {
			a.loadCover(coverKey, coverURL)
		}
		// hover 不改封面（不加遮罩、不加描边），只让下方标题变粉
		if box.Clicked() && onTap != nil {
			onTap()
		}
		// 标题（主）+ 副标题（次）：hover 变粉（同最近播放效果）
		titleCol := clrText
		if box.Hovered() {
			titleCol = clrPink
		}
		ui.Text(c, title).FontSize(12 * uiScale).TextColor(titleCol).MaxLines(1)
		if subtitle != "" {
			ui.Text(c, subtitle).FontSize(10 * uiScale).TextColor(clrMuted).MaxLines(1)
		}
	})
}

func (a *app) recentCard(c *ui.Context, item bilibili.SearchItem) {
	// 封面（方形，随网格宽度自适应）— 复用 gridCard 共享样式
	size := a.recentCardSize()
	a.gridCard(c, item.BVID, item.Pic, bilibili.CleanHTML(item.Title), item.Author, size, func() {
		go a.play(item)
	})
}

// recentCardSize 网格卡片封面边长（桌面 6 列时 ~120，紧凑/手机随列数自适应）。
func (a *app) recentCardSize() float32 {
	// 窗口宽 / 列数，减去间距（12*2），留封面方边。用固定档位避免过度计算。
	switch a.tier() {
	case 0:
		return a.resp(96, 88)
	case 1:
		return a.resp(112, 96)
	default:
		return a.resp(132, 112)
	}
}

// currentToItem 把当前曲目转回 SearchItem（供 play 复用）。
func (a *app) currentToItem() *bilibili.SearchItem {
	if a.current == nil {
		return nil
	}
	return &bilibili.SearchItem{BVID: a.current.bvid, Title: a.current.title, Author: a.current.author, Pic: a.current.pic}
}

// queueRow 队列行：序号 + 封面 + 标题/作者 + 当前播放高亮。
// 交互对齐搜索行（listRow）：单击展开/收起完整标题，双击播放（不是单击就播）。
// queueRow 队列行（复用 songRow）：序号 + 当前播放高亮 + ✕ 移除；单击播放 + 右键菜单。
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
func (a *app) favPage(c *ui.Context) {
	// 歌单详情（点歌单卡进入）优先
	if a.songlistOpen != 0 {
		a.songlistDetail(c)
		return
	}
	// 整页滚动条（对齐主线）：整页内容包一个 Scroll，不做组件内部滚动条
	ui.Scroll(c).Grow(1).Scrollbars(true).Children(func() {
		ui.Column(c).Fill().PaddingX(a.resp(24, 12)).PaddingY(a.resp(16, 10)).Gap(14 * uiScale).Children(func() {
			// ① 喜欢（原收藏）区
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
				ui.Text(c, "我喜欢的").FontSize(20 * uiScale).Bold().TextColor(clrText)
				ui.Text(c, fmt.Sprintf("%d 首", len(a.favorites))).FontSize(13 * uiScale).TextColor(clrMuted)
				ui.Spacer(c).Grow(1)
				if len(a.favorites) > 0 {
					if chip(c, "播放全部").Clicked() {
						a.playAll(a.favorites, 0)
					}
					if chip(c, "清空喜欢").Clicked() {
						a.svc.fav.Clear()
						a.refreshFavs()
						a.win.Invalidate()
					}
				}
			})
			if len(a.favorites) == 0 {
				ui.Column(c).Fill().Center().Gap(12 * uiScale).PaddingY(20 * uiScale).Children(func() {
					icon(c, icHeart, 44, ui.RGBA(0xff, 0xff, 0xff, 0.08))
					ui.Text(c, "喜欢的歌还空着").FontSize(17 * uiScale).Bold().TextColor(clrText)
					ui.Text(c, "听歌时点卡片上的 ♥ 收藏").FontSize(13 * uiScale).TextColor(clrMuted)
				})
			} else {
				ui.Column(c).Gap(6 * uiScale).Children(func() {
					favPage := a.pg["fav"]
					for _, item := range pageSlice(a.favorites, favPage) {
						a.favRow(c, item)
					}
					pager(c, favPage, pageCount(len(a.favorites)), func(p int) {
						a.pg["fav"] = p
						a.win.Invalidate()
					})
				})
			}
			// ② 歌单区（对齐主线 songlist_grid.html）
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).PaddingY(6 * uiScale).Children(func() {
				ui.Text(c, "我的歌单").FontSize(20 * uiScale).Bold().TextColor(clrText)
				ui.Text(c, fmt.Sprintf("%d 份", len(a.songlists))).FontSize(13 * uiScale).TextColor(clrMuted)
				ui.Spacer(c).Grow(1)
				if gradButton(c, "新建歌单").Clicked() {
					a.songlistNewOpen = true
					a.songlistNameInput = ""
					a.win.Invalidate()
				}
			})
			if a.songlistNewOpen {
				a.songlistNewRow(c)
			}
			if len(a.songlists) == 0 && !a.songlistNewOpen {
				ui.Column(c).Fill().Center().Gap(12 * uiScale).PaddingY(20 * uiScale).Children(func() {
					icon(c, icList, 44, ui.RGBA(0xff, 0xff, 0xff, 0.08))
					ui.Text(c, "还没有歌单").FontSize(17 * uiScale).Bold().TextColor(clrText)
					ui.Text(c, "把喜欢的歌归类收藏").FontSize(13 * uiScale).TextColor(clrMuted)
				})
			} else if len(a.songlists) > 0 {
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
			}
		})
	})
}

// favRow 收藏行。
// favRow 收藏行（复用 songRow）：♥ 取消收藏；单击播放 + 右键菜单。
func (a *app) favRow(c *ui.Context, item bilibili.SearchItem) {
	a.songRow(c, item.BVID, item.Pic, item.Title, item.Author, item.Duration, songRowOpt{
		isFav: true,
		heartFn: func() {
			a.svc.fav.Remove(item.BVID)
			a.refreshFavs()
			a.win.Invalidate()
		},
	})
}

// settingsPage 设置页：全量对齐主线 settings.js 渲染的 11 个区块
// （主题模式/网页字幕条/音质/播放模式/歌词字号/歌词背景/歌词时间/字幕字号/歌曲缓存/歌词缓存/数据/关于）。
func (a *app) settingsPage(c *ui.Context) {
	ui.Column(c).Fill().Padding(a.resp(16, 10), a.resp(24, 12), 0, a.resp(24, 12)).Gap(10 * uiScale).Children(func() {
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "设置").FontSize(20 * uiScale).Bold().TextColor(clrText)
			ui.Spacer(c).Grow(1)
			ui.Text(c, "自动 ／ 明亮 ／ 暗黑（点击切换）").FontSize(12 * uiScale).TextColor(clrMuted)
		})
		ui.Scroll(c).Grow(1).Gap(10 * uiScale).Scrollbars(true).Children(func() {
			// 限宽居中（对齐主线 max-w-2xl mx-auto）：宽屏不撑满，条项整体居中
			ui.Column(c).Gap(10 * uiScale).AlignSelf(ui.Center).WidthPercent(100).MaxWidth(672 * uiScale).Children(func() {
				// 1 主题模式（主线 THEMES 三态：自动/明亮/暗黑；持久化 settings kv）
				a.setRow(c, "主题模式")
				a.setRowSeg(c, []string{"自动", "明亮", "暗黑"}, a.themeMode, func(i int) {
					a.themeMode = i
					a.savePref("mygo_theme", i)
					a.win.Invalidate()
				})
				// 2 音质（主线 d-quality / set-q 三档）
				a.setRow(c, "音质")
				a.setRowSeg(c, []string{"低", "中", "高"}, a.quality, func(i int) {
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
					a.savePref("mygo_lyricsize", i)
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
	ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).PaddingX(4 * uiScale).Children(func() {
		ui.Text(c, label).FontSize(15 * uiScale).Bold().TextColor(clrText)
		ui.Spacer(c).Grow(1)
	})
}

// setRowText 区块内说明文字行（对齐主线 text-xs text-gray-500）。
func (a *app) setRowText(c *ui.Context, txt string) {
	ui.Text(c, txt).FontSize(12 * uiScale).TextColor(clrMuted).PaddingX(4 * uiScale)
}

// setRowSeg 分段按钮行（对齐主线 flex gap-2 圆角按钮组）。
func (a *app) setRowSeg(c *ui.Context, opts []string, cur int, cb func(int)) {
	ui.Row(c).Gap(a.resp(10, 8)).PaddingX(4 * uiScale).Wrap().Children(func() {
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
	ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).PaddingX(4 * uiScale).Children(func() {
		ui.Text(c, label).FontSize(14 * uiScale).Bold().TextColor(clrText)
		ui.Spacer(c).Grow(1)
		if switchToggle(c, on).Clicked() {
			cb(!on)
		}
	})
	ui.Text(c, hint).FontSize(11 * uiScale).TextColor(clrMuted).PaddingX(4 * uiScale)
}

// setRowBtns 操作按钮行（对齐主线 清空xxx 描边小按钮）。
func (a *app) setRowBtns(c *ui.Context, labels []string, cb func(int)) {
	ui.Row(c).Gap(8 * uiScale).PaddingX(4 * uiScale).Children(func() {
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
		a.svc.cache.ClearUnfav(fav.FavTable) // 收藏表名常量（fav 域包；保留收藏的缓存）
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
	ui.Box(c).Fill().Height(56 * uiScale).Radius(12 * uiScale).PaddingX(16 * uiScale).Background(clrCard).Children(func() {
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Children(func() {
			ui.Text(c, label).FontSize(a.resp(15, 14)).TextColor(clrText).Bold().Width(a.resp(140, 104))
			ui.Text(c, value).FontSize(13 * uiScale).TextColor(clrMuted)
			ui.Spacer(c).Grow(1)
			if hint != "" {
				ui.Text(c, hint).FontSize(12 * uiScale).TextColor(clrMuted)
			}
		})
	})
}

// settingRowSwitch 开关行（对齐主线网页字幕条开关）。
func (a *app) settingRowSwitch(c *ui.Context, label string, on bool, hint string, cb func(bool)) {
	ui.Box(c).Fill().Height(56 * uiScale).Radius(12 * uiScale).PaddingX(16 * uiScale).Background(clrCard).Children(func() {
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Children(func() {
			ui.Text(c, label).FontSize(a.resp(15, 14)).TextColor(clrText).Bold().Width(a.resp(140, 104))
			ui.Text(c, hint).FontSize(13 * uiScale).TextColor(clrMuted)
			ui.Spacer(c).Grow(1)
			if switchToggle(c, on).Clicked() {
				cb(!on)
			}
		})
	})
}

// settingRowSeg 分段选择行（对齐主线 settings 的音质/播放模式按钮组）。
func (a *app) settingRowSeg(c *ui.Context, label string, opts []string, cur int, cb func(int)) {
	ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(16 * uiScale).PaddingY(10 * uiScale).Background(clrCard).Children(func() {
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Children(func() {
			ui.Text(c, label).FontSize(a.resp(15, 14)).TextColor(clrText).Bold().Width(a.resp(140, 104))
			ui.Row(c).Gap(6 * uiScale).Children(func() {
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
	box := ui.Box(c).Size(46, 26).Radius(13 * uiScale)
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		if on {
			p.FillGradient(r, grad(), 13)
		} else {
			p.Fill(r, clrField, 13)
		}
	})
	box.Children(func() {
		knob := ui.Box(c).Size(20, 20).Radius(10 * uiScale).Absolute().Top(3)
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
	box := ui.Box(c).Height(32 * uiScale).Radius(8 * uiScale).PaddingX(12 * uiScale).AlignItems(ui.Center).Justify(ui.Center)
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		if on {
			p.FillGradient(r, grad(), 8)
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
		ui.Text(c, label).FontSize(13 * uiScale).TextColor(col)
	})
	return box
}

// castPage 投屏/音箱页：DLNA 设备发现 + 投当前歌 + 小米音箱（SSOT：cast/speaker 域包）。
func (a *app) castPage(c *ui.Context) {
	// 整页滚动条（对齐主线页面滚动条）：整页内容包一个 Scroll，内容超出窗口
	// 时整页滚动；不做组件内部滚动条（DLNA 列表不再单独 Scroll）。
	ui.Scroll(c).Grow(1).Scrollbars(true).Children(func() {
		ui.Column(c).Fill().PaddingX(a.resp(24, 12)).PaddingY(a.resp(16, 10)).Gap(12 * uiScale).Children(func() {
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
				ui.Text(c, "投屏 / 音箱").FontSize(20 * uiScale).Bold().TextColor(clrText)
				ui.Spacer(c).Grow(1)
				if gradButton(c, "发现设备").Clicked() {
					go a.discoverCast()
				}
			})
			// 当前投屏状态（对齐主线 d-stop：投屏中可停止）
			if a.castOn != "" {
				ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(16 * uiScale).PaddingY(12 * uiScale).Background(clrCard).Children(func() {
					ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(12 * uiScale).Children(func() {
						icon(c, icCast, 18, clrPink)
						ui.Text(c, "正在投屏："+a.castOn).FontSize(14 * uiScale).TextColor(clrText).Bold()
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
				ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(16 * uiScale).PaddingY(12 * uiScale).Background(clrCard).Children(func() {
					ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
						icon(c, icMsg, 18, clrPurple)
						ui.Column(c).Grow(1).Gap(2 * uiScale).Children(func() {
							ui.Text(c, "小爱音箱").FontSize(14 * uiScale).Bold().TextColor(clrText)
							uid := a.svc.speakerUserID()
							ui.Text(c, "已登录 · "+uid).FontSize(11 * uiScale).TextColor(clrMuted)
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
					ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(16 * uiScale).PaddingY(12 * uiScale).Background(clrCard).Children(func() {
						ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(16 * uiScale).Children(func() {
							// 二维码图（UI 解码 data URI）
							ui.Box(c).Size(120, 120).Draw(func(p *ui.Painter, r ui.Rect) {
								// 背景先画（否则后面 FillRadii 背景色会盖住二维码图）
								p.FillRadii(r, [4]float32{8, 8, 8, 8}, clrCard)
								if img, ok := a.covers["__qr__"]; ok {
									p.Clip(r, 8, func() { p.Image(img, r, ui.Cover) })
								}
							})
							ui.Column(c).Grow(1).Gap(6 * uiScale).Children(func() {
								ui.Text(c, "小米音箱扫码登录").FontSize(14 * uiScale).Bold().TextColor(clrText)
								ui.Text(c, "用米家 App 扫描二维码登录").FontSize(12 * uiScale).TextColor(clrMuted)
								ui.Text(c, a.qrStatus).FontSize(12 * uiScale).TextColor(clrPink)
							})
						})
					})
				}
				ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(16 * uiScale).PaddingY(10 * uiScale).Background(clrCard).Children(func() {
					ui.Column(c).Gap(8 * uiScale).Children(func() {
						ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
							icon(c, icMsg, 18, clrMuted)
							ui.Text(c, "小爱音箱未登录").FontSize(13 * uiScale).TextColor(clrText).Grow(1)
						})
						// token 输入框（对齐主线 speaker_login.html sp-token-input textarea）：
						// 用户手动粘贴/输入 token（多端免扫码复用）；也支持剪贴板一键填入。
						ui.TextInput(c, &a.tokenInput).Grow(1).Height(60 * uiScale).
							Placeholder("粘贴音箱 token（其他设备导出）…").
							FontSize(12 * uiScale).TextColor(clrText)
						ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
							ui.Spacer(c).Grow(1)
							// 读剪贴板填入输入框（快捷）
							if chip(c, "粘贴剪贴板").Clicked() {
								go a.fillTokenFromClipboard()
							}
							// 用输入框 token 登录（对齐主线 textarea 值 → POST /speaker/token/import）
							if gradButton(c, "token 登录").Clicked() {
								go a.loginWithToken(a.tokenInput)
							}
							if gradButton(c, "扫码登录").Clicked() {
								go a.startSpeakerLogin()
							}
						})
					})
				})
			}
			// 音箱设备列表（登录后展示，SSOT speaker）——对齐主线 speaker_devices.html：
			// 网格卡片（图标+名称+硬件型号+在线/离线徽标+「投送当前歌」+「停止」），非单行列表。
			if loggedIn {
				if len(a.speakers) == 0 {
					ui.Text(c, "音箱：点「刷新设备」获取设备列表").FontSize(12 * uiScale).TextColor(clrMuted).PaddingX(4 * uiScale)
				}
				// 网格（2 列等分，对齐主线 grid sm:grid-cols-2）
				ui.Grid(c).ColumnTracks(ui.Fr(1), ui.Fr(1)).Gap(10 * uiScale).Fill().Children(func() {
					for _, sp := range a.speakers {
						sp := sp
						card := ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(14 * uiScale).PaddingY(12 * uiScale).Background(clrCard)
						card.Children(func() {
							ui.Column(c).Fill().Gap(8 * uiScale).Children(func() {
								// 头部：图标 + 名称/硬件 + 在线徽标（对齐主线 sp 卡片头部）
								ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
									icon(c, icVol, 18, clrMuted)
									ui.Column(c).Grow(1).Gap(2 * uiScale).Children(func() {
										ui.Text(c, sp.Name).FontSize(14 * uiScale).Bold().TextColor(clrText).MaxLines(1)
										// 硬件型号（对齐主线 {{.Hardware}}；旧版显示 DID 不对齐）
										ui.Text(c, sp.Hardware).FontSize(11 * uiScale).TextColor(clrMuted).MaxLines(1)
									})
									// 在线/离线徽标（对齐主线 sp 卡片右侧，复用 statusBadge）
									if sp.IsOnline {
										statusBadge(c, "在线", clrGreen)
									} else {
										statusBadge(c, "离线", clrMuted)
									}
								})
								// 按钮行：「投送」（粉色渐变，对齐主线 sp-play-btn）+「停止」
								ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Children(func() {
									if gradButton(c, "投送").Clicked() {
										go a.playToSpeaker(sp.DID, sp.Name)
									}
									if chip(c, "停止").Clicked() {
										go a.stopSpeaker(sp.DID)
									}
								})
								// 状态标签（对齐主线 speaker.js sp-state：播放中/已暂停/播完自动连播）
								if a.spkDid == sp.DID {
									var label string
									if a.playing {
										label = "▶ 播放中"
										if a.pb.NextBVID() != "" {
											label = "▶ 播放中 · 播完自动连播"
										}
									} else {
										label = "⏸ 已暂停"
									}
									ui.Text(c, label).FontSize(11 * uiScale).TextColor(clrPink).Bold()
								}
							})
						})
					}
				})
			}
			ui.Text(c, "DLNA 设备").FontSize(14 * uiScale).Bold().TextColor(clrText).PaddingX(4 * uiScale)
			if len(a.castDevices) == 0 {
				ui.Text(c, "点「发现设备」搜索同网段 DLNA 播放器/电视").FontSize(12 * uiScale).TextColor(clrMuted).PaddingX(4 * uiScale)
			}
			ui.Column(c).Gap(8 * uiScale).Children(func() {
				for _, d := range a.castDevices {
					d := d
					card := ui.Box(c).Fill().Radius(12 * uiScale).PaddingX(14 * uiScale).PaddingY(12 * uiScale).Background(clrCard)
					if card.Hovered() {
						card.Background(clrCardHov)
					}
					card.Children(func() {
						ui.Column(c).Fill().Gap(8 * uiScale).Children(func() {
							// 头部：图标 + 名称/IP + 状态徽标（对齐主线 cast-status span：在线/真实状态）
							ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
								icon(c, icCast, 18, clrMuted)
								ui.Column(c).Grow(1).Gap(2 * uiScale).Children(func() {
									ui.Text(c, d.Name).FontSize(14 * uiScale).Bold().TextColor(clrText).MaxLines(1)
									ui.Text(c, d.IP+":"+fmt.Sprintf("%d", d.Port)).FontSize(11 * uiScale).TextColor(clrMuted).MaxLines(1)
								})
								stateText, ok := a.castStatus[d.UDN]
								if !ok || stateText == "" {
									stateText = "在线"
								}
								statusBadge(c, stateText, clrGreen) // 复用状态徽标（原纯文字）
							})
							// 按钮行：「查状态」+「投送」（对齐主线 hx/cast/status + cast-play-btn）
							ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Children(func() {
								if chip(c, "查状态").Clicked() {
									go a.queryCastStatus(d.UDN)
								}
								if gradButton(c, "投送").Clicked() {
									go a.castTo(d.UDN, d.Name)
								}
							})
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
			// qr_image 是 "data:image/png;base64,..."（xiaoai qrcode.go 编码），
			// DecodeImage 只收原始 bytes，必须先去掉 data URI 前缀 + base64 解码，
			// 否则解码失败 → 二维码不显示。
			raw := []byte(qri)
			if i := strings.Index(qri, ","); i >= 0 {
				if b, derr := base64.StdEncoding.DecodeString(qri[i+1:]); derr == nil {
					raw = b
				}
			}
			if img, derr := ui.DecodeImage(raw); derr == nil {
				a.covers["__qr__"] = img
			} else {
				a.qrStatus = "二维码解码失败：" + derr.Error()
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

// importTokenFromClipboard 从剪贴板读 token 粘贴登录（对齐主线 speaker_login.html
// 的 sp-token-input 粘贴入口 + POST /api/v1/speaker/token/import）。
// fillTokenFromClipboard 读剪贴板填入 token 输入框（不登录，用户可编辑后再登录）。
func (a *app) fillTokenFromClipboard() {
	if a.svc == nil || a.svc.qrAuth == nil {
		return
	}
	data, err := mygo.Clipboard.Read(transfer.Text)
	if err != nil {
		a.win.Update(func() { a.status = "读取剪贴板失败：" + err.Error() })
		return
	}
	text, _ := data.Read(transfer.Text)
	tok := strings.TrimSpace(string(text))
	a.win.Update(func() {
		a.tokenInput = tok
		if tok == "" {
			a.status = "剪贴板为空，请先复制音箱 token"
		}
		a.win.Invalidate()
	})
}

// loginWithToken 用输入的 token 登录（对齐主线 sp-token-input textarea 值 → import）。
func (a *app) loginWithToken(tok string) {
	if a.svc == nil || a.svc.qrAuth == nil {
		return
	}
	tok = strings.TrimSpace(tok)
	if tok == "" {
		a.win.Update(func() { a.status = "请输入音箱 token" })
		a.win.Invalidate()
		return
	}
	if err := a.svc.importSpeakerToken(tok); err != nil {
		a.win.Update(func() { a.status = err.Error() })
		a.win.Invalidate()
		return
	}
	a.win.Update(func() { a.status = "token 登录成功，正在刷新设备…" })
	go a.refreshSpeakers()
	a.win.Invalidate()
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
// queryCastStatus 查 DLNA 设备播放状态（对齐主线 hx/cast/status：cast.Status(udn) → state）。
func (a *app) queryCastStatus(udn string) {
	if a.svc == nil || a.svc.cast == nil {
		return
	}
	status, err := a.svc.cast.Status(udn)
	if err != nil || status == nil {
		a.win.Update(func() { a.castStatus[udn] = "状态不可用" })
		return
	}
	state, _ := status["state"].(string)
	if state == "" {
		state = "就绪"
	}
	a.win.Update(func() {
		if a.castStatus == nil {
			a.castStatus = map[string]string{}
		}
		a.castStatus[udn] = state
	})
}

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
	// 自动连播：NextBVID 非空 = 音箱播完由 speaker 域包定时器触发 hook 切下一首
	// （对齐主线 hx/speaker.go：next 为空则 NextBVID() 取核心队列下一首；
	// 核心队列 SSOT 算 next，不依赖前端传参）。
	opts := speaker.PlayOpts{Title: a.current.title, Duration: 0, NextBVID: a.pb.NextBVID()}
	// 音质 mock：主线 hx/speaker.go 固定 30280（= bilibili.Audio192K，B站 qn 码，
	// 不是"192k"这个字面量）；旧版传 192 会走 GetBestAudioURL 回退链才拿到 30280。
	qn := bilibili.Audio192K
	if a.quality == 0 {
		qn = bilibili.Audio64K
	} else if a.quality == 1 {
		qn = bilibili.Audio128K
	}
	if res, err := a.svc.speaker.Play(a.current.bvid, did, qn, opts); err != nil {
		a.status = "音箱播放失败：" + err.Error()
	} else if code, _ := res["code"].(int); code == 0 {
		a.castOn = name
		a.status = "已在音箱播放：" + name
		// 投送成功 → 同步切核心 source=speaker（对齐主线 hx/speaker.go 注释：
		// “消除双响窗口——分两步时前端在‘音箱已响但核心还是 web/playing’的间隙
		// 让手机也开播”）。SwitchSpeaker 内 prevOut(本地 MyGoOutput).Stop() 停本地
		// 出声 + 状态切音箱 + SpeakerOutput 接管进度投影。缺它：音箱在响核心却说
		// web/playing，本地也在播 = 真双响。
		a.pb.SwitchSpeaker(did)
	}
	a.win.Invalidate()
}

// stopSpeaker 停止音箱投送（对齐主线 stop_speaker 命令 → playback.StopSpeaker 唯一实现：
// 音箱停 + source 切回 web；castOn 清空回“未投屏”态）。
func (a *app) stopSpeaker(did string) {
	if a.svc == nil || a.svc.speaker == nil {
		return
	}
	a.pb.StopSpeaker(did)
	a.castOn = ""
	a.status = "已停止投屏"
	a.win.Invalidate()
}

// openCtxMenu 打开右键菜单（对齐主线 contextmenu：点某行）
func (a *app) openCtxMenu(item *bilibili.SearchItem) {
	a.ctxItem = item
	a.ctxPosSet = false // 重置：ctxMenu 首帧记录一次鼠标位置，之后固定不跟鼠标
	a.win.Invalidate()
}

// ctxMenu 右键菜单浮层（对齐主线 contextmenu.js 四项：播放/下一首播放/收藏/复制链接）
func (a *app) ctxMenu(c *ui.Context) {
	item := *a.ctxItem
	// Overlay 层（mygo ui 专用置顶浮层，对齐 nowPlaying；否则被 Overlay 层盖住不置顶）
	ui.Overlay(c, func() {
		// 背景：点外部关闭 + 记录鼠标窗口位置（bg 从 0,0 铺满 → PointerPosition = 窗口坐标）
		bg := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0)
		bg.Draw(func(p *ui.Painter, r ui.Rect) {})
		mx, my, _ := bg.PointerPosition()
		// 首帧记录一次鼠标窗口位置（bg 从 0,0 铺满 → PointerPosition = 窗口坐标），
		// 之后固定不跟鼠标（原生右键菜单行为）。
		if !a.ctxPosSet {
			a.ctxX, a.ctxY = mx, my
			a.ctxPosSet = true
		}
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
			{"添加到歌单", func() { a.addToPick = true }}, // ctxItem 保留：浮层以它为目标准歌
			{"复制链接", func() { a.copyLink(item); a.ctxItem = nil }},
		}
		w := float32(160)
		h := float32(len(items))*38 + 8
		// 菜单固定在右键点击位置（a.ctxX/ctxY = 打开那一刻记录一次，不跟鼠标）
		lx, ly := a.ctxX, a.ctxY
		// 贴边防溢出
		if lx+w > float32(a.winW) {
			lx = float32(a.winW) - w
		}
		if ly+h > float32(a.winH) {
			ly = float32(a.winH) - h
		}
		menu := ui.Box(c).Absolute().Left(lx).Top(ly).Width(w).Height(h).Radius(12 * uiScale).Background(clrCard).PaddingY(4 * uiScale)
		menu.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Shadow(r, 16, 0, 10, 36, 0, ui.RGBA(0, 0, 0, 0.45))
			p.FillRadii(r, [4]float32{12, 12, 12, 12}, clrCard)
		})
		menu.Children(func() {
			ui.Column(c).Gap(0 * uiScale).Children(func() {
				for _, it := range items {
					it := it
					row := ui.Box(c).Fill().Height(38 * uiScale).PaddingX(12 * uiScale)
					if row.Hovered() {
						row.Background(clrField)
					}
					row.Children(func() {
						ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Children(func() {
							ui.Text(c, it.label).FontSize(13 * uiScale).TextColor(clrText)
						})
					})
					if row.Clicked() {
						it.fn()
						a.win.Invalidate()
					}
				}
			})
		})
	})
}

// playNext 下一首播放（对齐主线 play-next：插到当前歌后面 → 这里等价"立即播完这首接它"，
// MyGo 用 SetQueue 语义：取当前队列，把 item 插到 playIndex+1，重新 SetQueue）
func (a *app) playNext(item bilibili.SearchItem) {
	// 对齐主线：右键"下一首播放"= play-next → hcState.cmd('play') → Pb.Play。
	// 主线 Pb.Play 语义=去重后入队尾+定位播放（无"插到当前歌后"的插队概念）；
	// 旧版自创 SetQueue 手动插队（与主线不一致）。直接走 SSOT Play。
	go a.pb.Play(toSong(item))
}

// copyLink 复制 B站链接到剪贴板（mygo Clipboard.WriteText）。
func (a *app) copyLink(item bilibili.SearchItem) {
	url := "https://www.bilibili.com/video/" + item.BVID
	mygo.Clipboard.WriteText(url)
	a.status = "已复制：" + url
	a.win.Invalidate()
}
