//go:build native

package main

import (
	"fmt"
	"homecast/service/fav"
	"runtime"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"mymusic-mygo/widgetoverlay"
)

func (a *app) settingsPage(c *ui.Context) {
	ui.Column(c).Fill().Padding(a.resp(16, 10), a.resp(24, 12), a.resp(12, 8), a.resp(24, 12)).Gap(10 * uiScale).Children(func() {
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "设置").FontSize(20 * uiScale).Bold().TextColor(clrText)
			ui.Spacer(c).Grow(1)
			// 子 tab：系统设置 / 插件（同一导航项分页显示）
			a.setRowSeg(c, []string{"系统设置", "插件", "日志"}, a.settingsTab, func(i int) {
				a.settingsTab = i
				a.syncLogWatch()
				a.win.Invalidate()
			})
		})
		switch a.settingsTab {
		case 1:
			a.pluginPage(c)
		case 2:
			a.logPage(c)
		default:
			a.systemSettings(c)
		}
	})
}

// systemSettings 系统设置内容（原设置页，主题/音质/缓存等）。
func (a *app) systemSettings(c *ui.Context) {
	ui.Scroll(c).Grow(1).Gap(10 * uiScale).Scrollbars(false).Children(func() {
		// 限宽居中（对齐主线 max-w-2xl mx-auto）：宽屏不撑满，条项整体居中
		ui.Column(c).Gap(10 * uiScale).AlignSelf(ui.Center).WidthPercent(100).MaxWidth(672 * uiScale).Children(func() {
			// 1 主题模式（主线 THEMES 三态：自动/明亮/暗黑；持久化 settings kv）
			a.setRow(c, "主题模式")
			a.setRowSeg(c, []string{"自动", "明亮", "暗黑"}, a.themeMode, func(i int) {
				a.themeMode = i
				a.savePref("mygo_theme", i)
				a.win.Invalidate()
			})
			// 4 播放模式
			a.setRow(c, "播放模式")
			a.setRowSeg(c, []string{"顺序播放", "列表循环", "单曲循环", "随机播放"}, a.loopMode, func(i int) {
				a.loopMode = i
				a.pb.SetMode(intToMode(i))
			})
			// 点单曲时：替换播放列表（网易云默认）/ 仅追加到播放列表
			a.setRow(c, "点歌方式")
			a.setRowSeg(c, []string{"替换列表", "追加列表"}, a.pointPlayMode, func(i int) {
				a.pointPlayMode = i
				a.savePref("mygo_pointplay", i)
				a.win.Invalidate()
			})
			// 9 歌曲缓存：主标题 + 已用/首数 + 清空；上限作为次级行（不另起白字主标题）
			a.setRow(c, "歌曲缓存")
			a.setRowText(c, "已用 "+a.cacheUsedLabel()+" · "+a.cacheCountLabel()+" 首")
			a.setRowBtns(c, []string{"清空未收藏", "清空缓存"}, func(i int) {
				if i == 0 {
					a.clearCache(false)
				} else {
					a.clearCache(true)
				}
			})
			a.limitSubRow(c, "上限", []string{"关闭", "200MB", "500MB", "1GB", "自定义"},
				a.cacheLimit, &a.cacheCustom, "MB", func(idx int, custom int) {
					a.cacheLimit = idx
					a.cacheCustom = custom
					a.savePref("mygo_cachelimit", idx)
					a.savePref("mygo_cachelimit_custom", custom)
					if a.svc != nil && a.svc.cache != nil {
						a.svc.cache.SetLimitMB(a.cacheMB())
					}
				})
			// 10 歌词缓存：同样结构，数值档位低一档
			a.setRow(c, "歌词缓存")
			a.setRowText(c, "已用 "+a.lyricCacheSize()+" · "+a.lyricCacheCount()+" 首")
			a.setRowBtns(c, []string{"清空"}, func(int) {
				if a.svc != nil && a.svc.lyricCache != nil {
					a.svc.lyricCache.ClearAll()
					a.win.Invalidate()
				}
			})
			a.limitSubRow(c, "上限", []string{"关闭", "100 首", "300 首", "500 首", "自定义"},
				a.lyricLimit, &a.lyricCustom, "首", func(idx int, custom int) {
					a.lyricLimit = idx
					a.lyricCustom = custom
					a.savePref("mygo_lyriclimit", idx)
					a.savePref("mygo_lyriclimit_custom", custom)
					if a.svc != nil && a.svc.lyricCache != nil {
						a.svc.lyricCache.SetLimit(a.lyricCap())
					}
				})
			// 11 歌词挂件控制条位置
			a.setRow(c, "歌词挂件控制条")
			a.setRowSeg(c, []string{"上方", "下方"}, boolIdx(a.lyricCtrlBottom), func(i int) {
				a.lyricCtrlBottom = i == 1
				a.savePref("mygo_lyricctrlpos", i)
				a.win.Invalidate()
			})
			// 11.5 安卓悬浮歌词（仅 Android 有 Kotlin 实现；桌面不显示）
			if runtime.GOOS == "android" {
				a.setRow(c, "悬浮歌词")
				a.setRowSeg(c, []string{"关闭", "开启"}, boolIdx(a.androidLyricOn), func(i int) {
					on := i == 1
					if on {
						if !widgetoverlay.TryStart() {
							on = false // 未授权：已拉起设置页，用户授权后再开
						}
					} else {
						widgetoverlay.Stop()
					}
					a.androidLyricOn = on
					a.savePref("mygo_androlyric", i)
					a.win.Invalidate()
				})
			}
			// 12 启动恢复（详情藏弹窗，不堆在设置页）——放在更新上面
			a.restoreConfigEntry(c)
			// 13 更新（运行期自填源：GitHub / 自建 URL + 代理；校验公钥构建期烙进二进制）
			a.updateSettings(c)
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
	return float32([]int{14, 16, 18, 22, 26}[a.lyricSize]) * uiScale
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

// cacheCountLabel 歌曲缓存条目数。
func (a *app) cacheCountLabel() string {
	if a.svc == nil || a.svc.cache == nil {
		return "0"
	}
	return fmt.Sprintf("%d", a.svc.cache.Count())
}

// limitSubRow 上限次级行：灰字小标签单独一行，档位 seg 在下一行（与上方区块左对齐）。
// 不选 setRow 白字主标题——上限从属于上面的缓存区块，是次级设置。
func (a *app) limitSubRow(c *ui.Context, label string, opts []string, cur int, custom *int, unit string, apply func(int, int)) {
	ui.Column(c).AlignSelf(ui.Stretch).Gap(6 * uiScale).Children(func() {
		// 标签单独一行（跟"已用""清空"行左对齐，不缩进）
		ui.Row(c).AlignSelf(ui.Stretch).PaddingX(4 * uiScale).Children(func() {
			ui.Text(c, label).FontSize(12 * uiScale).TextColor(clrMuted)
		})
		// 档位 seg 下一行（不坏规矩：选项按钮独占一行）
		a.setRowSeg(c, opts, cur, func(i int) {
			// 所有档位都进确认弹窗（包括预置档），确定才生效
			a.limitKind = labelKind(unit)
			a.limitPendingIdx = i
			a.limitPendingVal = *custom
			a.limitBuf = fmt.Sprintf("%d", *custom)
			a.showLimitDlg = true
			a.win.Invalidate()
		})
		// 自定义已选时显示当前值（点击重新弹窗改）
		if cur == 4 {
			ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).PaddingX(4 * uiScale).Children(func() {
				ui.Text(c, fmt.Sprintf("自定义：%d %s", *custom, unit)).FontSize(11 * uiScale).TextColor(clrMuted)
				ui.Spacer(c).Grow(1)
				if chip(c, "修改").Clicked() {
					a.limitKind = labelKind(unit)
					a.limitPendingIdx = 4
					a.limitPendingVal = *custom
					a.limitBuf = fmt.Sprintf("%d", *custom)
					a.showLimitDlg = true
					a.win.Invalidate()
				}
			})
		}
	})
}

// customInputBuf 自定义档位首次选中时的初始值（不从输入读，直接给当前值）。
func (a *app) customInputBuf(_ int, custom *int) int { return *custom }

// parseIntDefault 解析失败回退默认值。
func parseIntDefault(s string, def int) int {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return v
}

// boolIdx bool → 0/1 索引（设置页 seg 用）。
func boolIdx(b bool) int {
	if b {
		return 1
	}
	return 0
}

// labelKind 用单位区分缓存类型（MB=歌曲缓存，首=歌词缓存）。
func labelKind(unit string) string {
	if unit == "MB" {
		return "cache"
	}
	return "lyric"
}

// customLimitDialog 自定义上限弹窗：输入数值 → 确定/取消（每个数值必须点确定才生效）。
// limitDialog 上限确认弹窗：所有档位（含预置）都经它，确定才生效。
// 精致版：色条标题 + 大数值卡（当前→新值 / 自定义输入）+ 渐变主按钮。
func (a *app) customLimitDialog(c *ui.Context) {
	if !a.showLimitDlg {
		return
	}
	cache := a.limitKind == "cache"
	unit, title := "首", "歌词缓存"
	if cache {
		unit, title = "MB", "歌曲缓存"
	}
	custom := a.limitPendingIdx == 4
	// 预置档显示值（0=关闭 1=200/100 2=500/300 3=1GB/500）
	presetVal := 0
	if !custom {
		if cache {
			presetVal = []int{0, 200, 500, 1024}[a.limitPendingIdx]
		} else {
			presetVal = []int{0, 100, 300, 500}[a.limitPendingIdx]
		}
	}
	lx, ly, pw, ph := a.dialogRect(360, 230)
	ui.Overlay(c, func() {
		bg := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0)
		bg.Draw(func(p *ui.Painter, r ui.Rect) { p.Fill(r, ui.RGBA(0, 0, 0, 0.55), 0) })
		if bg.Clicked() {
			a.showLimitDlg = false
			a.win.Invalidate()
		}
		panel := ui.Box(c).Absolute().Left(lx).Top(ly).Width(pw).Height(ph).Radius(18 * uiScale).Background(clrCard)
		panel.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Shadow(r, 24, 0, 14, 48, 0, ui.RGBA(0, 0, 0, 0.6))
			p.FillRadii(r, [4]float32{18, 18, 18, 18}, clrCard)
		})
		panel.Children(func() {
			ui.Column(c).Fill().Padding(20 * uiScale).Gap(12 * uiScale).Children(func() {
				// 标题行：色条 + 标题 + 关闭
				ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(9 * uiScale).Children(func() {
					ui.Box(c).Size(4*uiScale, 16*uiScale).Radius(2*uiScale).Background(clrPink)
					ui.Text(c, title+"上限").FontSize(16 * uiScale).Bold().TextColor(clrText)
					ui.Spacer(c).Grow(1)
					if chip(c, "×").Clicked() {
						a.showLimitDlg = false
						a.win.Invalidate()
					}
				})
				// 数值卡片：自定义=输入框，预置=大数值
				card := ui.Box(c).AlignSelf(ui.Stretch).Radius(12 * uiScale).Padding(12 * uiScale).Background(clrField)
				card.Children(func() {
					if custom {
						ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
							ui.Text(c, "自定义").FontSize(12 * uiScale).TextColor(clrMuted)
							ui.Spacer(c).Grow(1)
							ui.TextInput(c, &a.limitBuf).Width(90 * uiScale).Height(38 * uiScale).
								FontSize(22 * uiScale).TextColor(clrText).Background(ui.RGBA(0, 0, 0, 0))
							ui.Text(c, unit).FontSize(13 * uiScale).TextColor(clrPink)
						})
					} else {
						ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
							ui.Column(c).Gap(2 * uiScale).Children(func() {
								ui.Text(c, "上限设为").FontSize(11 * uiScale).TextColor(clrMuted)
								if presetVal == 0 {
									ui.Text(c, "关闭").FontSize(24 * uiScale).Bold().TextColor(clrText)
								} else {
									ui.Text(c, fmt.Sprintf("%d %s", presetVal, unit)).FontSize(24 * uiScale).Bold().TextColor(clrText)
								}
							})
						})
					}
				})
				ui.Spacer(c).Grow(1)
				// 按钮行：取消（描边）+ 确定（渐变）
				ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).Children(func() {
					ui.Spacer(c).Grow(1)
					if chip(c, "取消").Clicked() {
						a.showLimitDlg = false
						a.win.Invalidate()
					}
					if gradButton(c, "确定").Clicked() {
						v := a.limitPendingVal
						if custom {
							v = parseIntDefault(a.limitBuf, a.limitPendingVal)
							if v < 0 {
								v = 0
							}
						}
						idx := a.limitPendingIdx
						if cache {
							a.cacheLimit = idx
							a.cacheCustom = v
							a.savePref("mygo_cachelimit", idx)
							a.savePref("mygo_cachelimit_custom", v)
							if a.svc != nil && a.svc.cache != nil {
								a.svc.cache.SetLimitMB(a.cacheMB())
							}
						} else {
							a.lyricLimit = idx
							a.lyricCustom = v
							a.savePref("mygo_lyriclimit", idx)
							a.savePref("mygo_lyriclimit_custom", v)
							if a.svc != nil && a.svc.lyricCache != nil {
								a.svc.lyricCache.SetLimit(a.lyricCap())
							}
						}
						a.showLimitDlg = false
						a.win.Invalidate()
					}
				})
			})
		})
	})
}
