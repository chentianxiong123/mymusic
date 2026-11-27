//go:build native

package main

import (
	"fmt"
	"homecast/bilibili"
	"homecast/service/lyric"

	"github.com/egoist/mygo/ui"
)

func (a *app) nowPlaying(c *ui.Context) {
	ui.Overlay(c, func() {
		bg := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0)
		bg.Draw(func(p *ui.Painter, r ui.Rect) {
			// 背景：封面模糊（默认）/ 纯色（对齐主线 ly-bg-cover / ly-bg-solid）
			if a.lyricBg == 0 {
				// 背景：模糊版封面（降采样放大）优先；模糊未就绪→原图缩略兜底。
				if img, ok := a.blurCovers[a.current.bvid]; ok {
					p.Image(img, r, ui.Cover)
					p.Fill(r, ui.RGBA(0x0a, 0x0b, 0x10, 0.72), 0)
				} else if img, ok := a.covers[a.current.bvid]; ok {
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
					FontSize(11 * uiScale).TextColor(clrText).MaxLines(1).MaxWidth(120 * uiScale)
			})
			if len(a.lyricCands) == 0 {
				ui.Text(c, LaodingTag()).FontSize(12 * uiScale).TextColor(clrMuted).PaddingY(20 * uiScale)
			} else {
				ui.Scroll(c).Grow(1).Gap(4 * uiScale).Scrollbars(false).Children(func() {
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
										FontSize(11 * uiScale).TextColor(clrMuted).MaxLines(1)
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
				// 来源行（显示实际出词插件名；点击跳插件页配置歌词源绑定）
				srcRow := ui.Text(c, a.lyricSourceText()).FontSize(12 * uiScale).TextColor(clrPink).MaxLines(1)
				if srcRow.Clicked() {
					a.activeTab = "settings"
					a.settingsTab = 1
					a.pluginTab = 1
					a.win.Invalidate()
				}
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
			ui.Scroll(c).Fill().Gap(10 * uiScale).Scrollbars(false).Children(func() {
				ui.Column(c).AlignSelf(ui.Stretch).Gap(8 * uiScale).Children(func() {
					if len(a.lyricLines) == 0 {
						ui.Column(c).AlignSelf(ui.Stretch).Children(func() {
							ui.Text(c, "歌词加载中…").FontSize(15 * uiScale).TextColor(clrMuted).AlignSelf(ui.Center).PaddingY(40 * uiScale)
							ui.Text(c, "搜索："+lyric.CleanSearchKeyword(bilibili.CleanHTML(a.current.title))+" — 去「插件 → 歌词」启用更多歌词源").FontSize(11 * uiScale).TextColor(clrMuted).AlignSelf(ui.Center)
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
				if heartButton(c, a.isFav(), 26, func() {}).Clicked() {
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
				if lyricToggleButton(c, 32, a.lyricOn()).Clicked() {
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
						ui.Scroll(c).MaxHeight(200 * uiScale).Gap(4 * uiScale).Scrollbars(false).Children(func() {
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
						if heartButton(c, a.isFav(), 26, func() {}).Clicked() {
							a.toggleFav()
						}
					})
				})
			}
			// KTV 歌词（居中 + 渐底 + 点击跳句，同中栏）
			ui.Box(c).Grow(1).Fill().Radius(12 * uiScale).Background(clrCardGlass).PaddingX(12 * uiScale).PaddingY(10 * uiScale).Children(func() {
				curIdx, prog := a.lyricProg(a.posSec)
				ui.Scroll(c).Fill().Gap(8 * uiScale).Scrollbars(false).Children(func() {
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
