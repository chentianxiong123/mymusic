//go:build native

// log_page.go —— 设置页「日志」分页：结构化日志查看器（级别/分类/关键词筛选、
// 自动滚动、真清空、导出）+ 日志配置（容量/最低级别/写文件）。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// filterLogEntries 日志筛选（级别/分类/关键词）；抽函数便于回归测试。
// levelFilter: 0=全部 1=DEBUG 2=INFO 3=WARN 4=ERROR（e.Level 0..3 → +1）。
func filterLogEntries(entries []LogEntry, levelFilter int, cat, keyword string) []LogEntry {
	out := make([]LogEntry, 0, len(entries))
	for _, e := range entries {
		if levelFilter > 0 && int(e.Level)+1 != levelFilter {
			continue
		}
		if cat != "" && e.Category != cat {
			continue
		}
		if keyword != "" && !strings.Contains(strings.ToLower(e.Message), strings.ToLower(keyword)) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// logPage 日志查看器（设置页 tab=2）。
func (a *app) logPage(c *ui.Context) {
	entries := logEntries()
	capN, total, minLvl, writeFile, logPath := logStats()

	// 筛选（抽函数便于回归测试）
	filtered := filterLogEntries(entries, a.logLevelFilter, a.logCatFilter, a.logSearch)

	ui.Column(c).Fill().Gap(10 * uiScale).Children(func() {
		// ── 顶部工具栏 ──
		toolbar := ui.Box(c).AlignSelf(ui.Stretch).Radius(12 * uiScale).Padding(12 * uiScale).Background(clrField)
		toolbar.Children(func() {
			ui.Column(c).Fill().Gap(10 * uiScale).Children(func() {
				// 第一行：级别筛选 + 分类筛选 + 搜索框
				ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Wrap().Children(func() {
					ui.Text(c, "级别").FontSize(12 * uiScale).TextColor(clrMuted)
					a.setRowSeg(c, []string{"全部", "DEBUG", "INFO", "WARN", "ERROR"}, a.logLevelFilter, func(i int) {
						a.logLevelFilter = i
						a.win.Invalidate()
					})
					ui.Spacer(c).Grow(1)
					if chip(c, "清空日志").Clicked() {
						a.logConfirmClear = true
						a.win.Invalidate()
					}
					if chip(c, "导出").Clicked() {
						a.exportLog()
					}
				})
				// 第二行：分类下拉 + 关键词搜索（窄屏分两行，搜索框独占一行）
				if a.narrow() {
					ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Children(func() {
						a.logCategorySelect(c)
					})
					ui.TextInput(c, &a.logSearch).AlignSelf(ui.Stretch).Height(36 * uiScale).
						Placeholder("搜索日志内容").TextColor(clrText)
				} else {
					ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Children(func() {
						a.logCategorySelect(c)
						ui.Spacer(c).Grow(1)
						ui.TextInput(c, &a.logSearch).Grow(1).Height(34 * uiScale).
							Placeholder("搜索日志内容").TextColor(clrText)
					})
				}
			})
		})

		// ── 日志列表 ──（响应式：宽屏单行 / 窄屏两行；限制条数防卡顿；点行看全文）
		listCard := ui.Box(c).Fill().Grow(1).Radius(12 * uiScale).Padding(10 * uiScale).Background(clrField)
		listCard.Children(func() {
			if len(filtered) == 0 {
				ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Children(func() {
					ui.Text(c, "没有匹配的日志").FontSize(14 * uiScale).TextColor(clrMuted)
				})
				return
			}
			// 只渲染最近 N 条（默认 300，防止 1000 条全渲染卡顿）
			shown := filtered
			limit := a.logLimit
			if limit <= 0 {
				limit = 300
			}
			if len(shown) > limit {
				shown = shown[len(shown)-limit:]
			}
			narrow := a.narrow()
			ui.Scroll(c).Grow(1).Scrollbars(false).Children(func() {
				ui.Column(c).Fill().Gap(2 * uiScale).Children(func() {
					if len(shown) < len(filtered) {
						ui.Text(c, fmt.Sprintf("仅显示最近 %d 条（共 %d 条匹配）", len(shown), len(filtered))).
							FontSize(10 * uiScale).TextColor(clrMuted).AlignSelf(ui.Center)
					}
					for _, e := range shown {
						e := e
						row := ui.Box(c).AlignSelf(ui.Stretch).Radius(6 * uiScale).Padding(4*uiScale, 6*uiScale).Background(clrCard)
						if narrow {
							// 窄屏：第一行 时间+级别+分类，第二行消息（可两行）
							row.Children(func() {
								ui.Column(c).Fill().Gap(2 * uiScale).Children(func() {
									ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(6 * uiScale).Children(func() {
										ui.Text(c, e.Time.Format("15:04:05")).FontSize(10*uiScale).TextColor(clrMuted)
										ui.Text(c, e.Level.String()).FontSize(10*uiScale).Bold().TextColor(logLevelColor(e.Level))
										ui.Text(c, e.Category).FontSize(10*uiScale).TextColor(clrPink).Grow(1)
									})
									ui.Text(c, e.Message).FontSize(11*uiScale).TextColor(clrText).MaxLines(2)
								})
							})
						} else {
							// 宽屏：单行（时间/级别/分类固定宽 + 消息 Grow）
							row.Children(func() {
								ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Children(func() {
									ui.Text(c, e.Time.Format("15:04:05")).FontSize(11*uiScale).TextColor(clrMuted).Width(64*uiScale)
									ui.Text(c, e.Level.String()).FontSize(11*uiScale).Bold().TextColor(logLevelColor(e.Level)).Width(44*uiScale)
									ui.Text(c, e.Category).FontSize(11*uiScale).TextColor(clrPink).Width(88*uiScale).MaxLines(1)
									ui.Text(c, e.Message).FontSize(12*uiScale).TextColor(clrText).Grow(1).MaxLines(1)
								})
							})
						}
						if row.Clicked() {
							eCopy := e
							a.logDetail = &eCopy
						}
					}
				})
			})
		})

		// 日志全文弹窗（点行展开，响应式 dialogRect）
		if a.logDetail != nil {
			a.logDetailDialog(c)
		}

		// ── 底部状态 + 配置 ──
		ui.Box(c).AlignSelf(ui.Stretch).Radius(12 * uiScale).Padding(12 * uiScale).Background(clrField).Children(func() {
			ui.Column(c).Fill().Gap(10 * uiScale).Children(func() {
				ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Wrap().Children(func() {
					counts := logCountByLevel()
					ui.Text(c, fmt.Sprintf("%d/%d 条 · 错误 %d · 警告 %d · 容量 %d", len(filtered), total, counts[logError], counts[logWarn], capN)).
						FontSize(11 * uiScale).TextColor(clrMuted)
					ui.Spacer(c).Grow(1)
					// 显示条数档位（防大列表卡顿）
					displayCaps := []int{100, 300, 1000, 0}
					displayLabels := []string{"100", "300", "1000", "全部"}
					curDisplay := 1
					for i, x := range displayCaps {
						if x == a.logLimit {
							curDisplay = i
						}
					}
					a.setRowSeg(c, displayLabels, curDisplay, func(i int) {
						a.logLimit = displayCaps[i]
						a.win.Invalidate()
					})
					if switchToggle(c, a.logAutoScroll).Clicked() {
						a.logAutoScroll = !a.logAutoScroll
						a.syncLogWatch()
						a.win.Invalidate()
					}
					ui.Text(c, "自动刷新").FontSize(11 * uiScale).TextColor(clrMuted)
				})
				// 配置行
				ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Wrap().Children(func() {
					ui.Text(c, "记录级别").FontSize(12 * uiScale).TextColor(clrMuted)
					cur := int(minLvl)
					a.setRowSeg(c, logLevelNames, cur, func(i int) {
						logSetLevel(LogLevel(i))
						a.saveLogConfig()
						a.win.Invalidate()
					})
					ui.Spacer(c).Grow(1)
					ui.Text(c, "写文件").FontSize(12 * uiScale).TextColor(clrMuted)
					if switchToggle(c, writeFile).Clicked() {
						logSetWriteFile(!writeFile)
						a.saveLogConfig()
						a.win.Invalidate()
					}
				})
				// 容量档位
				ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Wrap().Children(func() {
					ui.Text(c, "保留条数").FontSize(12 * uiScale).TextColor(clrMuted)
					caps := []int{200, 500, 1000, 2000, 5000}
					labels := []string{"200", "500", "1000", "2000", "5000"}
					curCap := 0
					for i, x := range caps {
						if x == capN {
							curCap = i
						}
					}
					a.setRowSeg(c, labels, curCap, func(i int) {
						logSetCapacity(caps[i])
						a.saveLogConfig()
						a.win.Invalidate()
					})
				})
				if logPath != "" {
					ui.Text(c, "文件："+logPath).FontSize(10 * uiScale).TextColor(clrMuted).MaxLines(1)
				}
			})
		})
	})

	// ── 清空确认弹窗（响应式 dialogRect）──
	if a.logConfirmClear {
		overlayClearConfirm(c, a)
	}
}

func logLevelColor(l LogLevel) ui.Color {
	switch l {
	case logError:
		return ui.RGBA(255, 97, 97, 1)
	case logWarn:
		return ui.RGBA(255, 199, 77, 1)
	case logDebug:
		return clrMuted
	}
	return clrText
}

// syncLogWatch 仅在「设置页 + 日志 tab」可见时开 1s 刷新；切走立即停（省电）。
func (a *app) syncLogWatch() {
	should := a.activeTab == "settings" && a.settingsTab == 2 && a.logAutoScroll
	if should == a.logWatching {
		return
	}
	a.logWatching = should
	if should {
		stop := make(chan struct{})
		a.logWatchStop = stop
		go func() {
			t := time.NewTicker(1 * time.Second)
			defer t.Stop()
			for {
				select {
				case <-stop:
					return
				case <-t.C:
					a.win.Invalidate()
				}
			}
		}()
	} else if a.logWatchStop != nil {
		close(a.logWatchStop)
		a.logWatchStop = nil
	}
}

// exportLog 导出日志到用户下载目录（时间戳文件名）。
func (a *app) exportLog() {
	dir, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dl := filepath.Join(dir, "下载")
	if _, err := os.Stat(dl); err != nil {
		dl = dir
	}
	name := fmt.Sprintf("mymusic-log-%s.log", time.Now().Format("20060102-150405"))
	if err := logExport(filepath.Join(dl, name)); err != nil {
		logExplicit(logError, "log", "导出失败: %v", err)
	} else {
		logExplicit(logInfo, "log", "已导出 %s", filepath.Join(dl, name))
	}
}

// overlayClearConfirm 清空确认弹窗：真清（内存+文件），必须二次确认。
func overlayClearConfirm(c *ui.Context, a *app) {
	lx, ly, pw, ph := a.dialogRect(360, 200)
	ui.Overlay(c, func() {
		bg := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0)
		bg.Draw(func(p *ui.Painter, r ui.Rect) { p.Fill(r, ui.RGBA(0, 0, 0, 0.45), 0) })
		if bg.Clicked() {
			a.logConfirmClear = false
			a.win.Invalidate()
		}
		box := ui.Box(c).Absolute().Left(lx).Top(ly).Width(pw).Height(ph).Radius(14 * uiScale).Background(clrCard).AlignItems(ui.Center)
		box.Draw(func(p *ui.Painter, r ui.Rect) { p.Fill(r, clrCard, 14) })
		box.Children(func() {
			ui.Column(c).Fill().Padding(18 * uiScale).Gap(14 * uiScale).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "清空全部日志？").FontSize(16 * uiScale).Bold().TextColor(clrText)
				ui.Text(c, "将清除内存记录和日志文件，不可恢复").FontSize(12 * uiScale).TextColor(clrMuted)
				ui.Spacer(c).Grow(1)
				ui.Row(c).Gap(12 * uiScale).Children(func() {
					if chip(c, "取消").Clicked() {
						a.logConfirmClear = false
						a.win.Invalidate()
					}
					if chip(c, "确认清空").Clicked() {
						if err := logClear(); err != nil {
							logExplicit(logError, "log", "清空日志失败: %v", err)
						}
						a.logConfirmClear = false
						a.win.Invalidate()
					}
				})
			})
		})
	})
}

// logCategorySelect 分类下拉（固定高度弹层防跳动；窄屏/宽屏共用）。
func (a *app) logCategorySelect(c *ui.Context) {
	ui.Text(c, "分类").FontSize(12 * uiScale).TextColor(clrMuted)
	cats := append([]string{"全部"}, logCategories()...)
	catIdx := 0
	for i, x := range cats {
		if x == a.logCatFilter {
			catIdx = i
		}
	}
	sel := ui.SelectBase(c, &catIdx)
	sel.Trigger.Height(34 * uiScale).PaddingX(12 * uiScale).Draw(func(p *ui.Painter, r ui.Rect) {
		if sel.Trigger.Hovered() {
			p.FillRadii(r, [4]float32{8, 8, 8, 8}, clrCardHov)
		}
	}).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(6 * uiScale).Children(func() {
			ui.Text(c, cats[catIdx]).FontSize(12 * uiScale).TextColor(clrText)
			ui.Text(c, "▾").FontSize(11 * uiScale).TextColor(clrMuted)
		})
	})
	popupH := len(cats)*38 + 12
	if popupH > 320 {
		popupH = 320
	}
	sel.Popup(func(panel ui.Element) {
		panel.Padding(6 * uiScale).Background(clrCard).Radius(12 * uiScale).Height(float32(popupH) * uiScale)
		ui.Scroll(c).Grow(1).Scrollbars(false).Children(func() {
			for i, name := range cats {
				i, name := i, name
				item := sel.Item(i).Height(38 * uiScale).PaddingX(12 * uiScale)
				if item.Highlighted() {
					item.Background(clrCardHov)
				}
				item.Children(func() {
					col := clrText
					if i == catIdx {
						col = clrPink
					}
					ui.Text(c, name).FontSize(12 * uiScale).TextColor(col)
				})
				if item.Clicked() {
					catIdx = i
					if i == 0 {
						a.logCatFilter = ""
					} else {
						a.logCatFilter = name
					}
					a.win.Invalidate()
				}
			}
		})
	})
}
