//go:build native

package main

import "github.com/egoist/mygo/ui"

func (a *app) navBar(c *ui.Context) {
	if a.narrow() {
		a.navBarMobile(c)
	} else {
		a.navBarDesktop(c)
	}
	ui.Box(c).AlignSelf(ui.Stretch).Height(1 * uiScale).Draw(func(p *ui.Painter, r ui.Rect) {
		p.Fill(r, clrCard, 0)
	})
	if a.navOpen {
		a.navPanel(c)
	}
}

// navBarDesktop 桌面导航（主线 hidden md:flex 同款）：logo + 5 tab 整行。
func (a *app) navBarDesktop(c *ui.Context) {
	px := a.resp(20, 12)
	navGap := a.resp(6, 3)
	ui.Row(c).AlignSelf(ui.Stretch).Height(64 * uiScale).PaddingX(px).Gap(navGap).AlignItems(ui.Center).Children(func() {
		icon(c, icMusic, a.resp(24, 20), clrText)
		ui.Spacer(c).Width(a.resp(10, 4))
		if navTab(c, icList, "队列", a.activeTab == "queue", false).Clicked() {
			a.setTab("queue")
		}
		if navTab(c, icFav, "歌单", a.activeTab == "fav", false).Clicked() {
			a.setTab("fav")
		}
		if navTab(c, icSearch, "来源", a.activeTab == "search", false).Clicked() {
			a.setTab("search")
		}
		if navTab(c, icFav, "统计", a.activeTab == "stats", false).Clicked() {
			a.setTab("stats")
		}
		if navTab(c, icCast, "目标", a.activeTab == "cast", false).Clicked() {
			a.setTab("cast")
		}
		if navTab(c, icSet, "设置", a.activeTab == "settings", false).Clicked() {
			a.setTab("settings")
		}
	})
}

// navBarMobile 移动导航（主线 md:hidden 同款）：logo + 汉堡按钮（右），无多余开关。
func (a *app) navBarMobile(c *ui.Context) {
	ui.Row(c).AlignSelf(ui.Stretch).Height(56 * uiScale).PaddingX(12 * uiScale).Gap(10 * uiScale).AlignItems(ui.Center).Children(func() {
		icon(c, icMusic, 20, clrText)
		ui.Text(c, "myMusic").FontSize(15 * uiScale).Bold().TextColor(clrText)
		ui.Spacer(c).Grow(1)
		if iconButton2(c, icMenu, 34, clrMuted).Clicked() {
			a.navOpen = !a.navOpen
			a.win.Invalidate()
		}
	})
}

// navPanel 侧滑导航面板（主线 side-nav-panel 同款）：点背景关闭，选 tab 后收起。
func (a *app) navPanel(c *ui.Context) {
	ui.Overlay(c, func() {
		// 点外部关闭（透明遮罩，对齐 nav.js 的 click-outside）
		bg := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0)
		bg.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Fill(r, ui.RGBA(0, 0, 0, 0.35), 0)
		})
		if bg.Clicked() {
			a.navOpen = false
		}
		// 面板：右上浮层（与右上汉堡同侧，w-48 风格）。
		// 动态计算（原 Left(12).Top(64).Width(200*uiScale) 是主线 Tailwind left-3 top-16 w-48 逐字翻译，
		// 存在 3 处硬编码问题：①Top 未乘 uiScale → uiScale>1 盖导航栏；②Width 窄屏过大；③侧别与汉堡相反）。
		navH := 56 * uiScale                            // 移动导航栏实际高（navBarMobile Height(56*uiScale)）
		panelTop := navH + 8*uiScale                    // 导航栏下方 8px
		panelW := 200 * uiScale                         // w-48 基准宽
		if m := float32(a.winW) * 0.5; panelW > m {
			panelW = m // 窄屏限 50% 窗宽（360px 手机 → ≤180px，避免糊半屏）
		}
		panelH := float32(a.winH) - panelTop - 16*uiScale // 高度上限（防矮屏/横屏溢出）
		if panelH < 200*uiScale { // winH 未就绪/窗口过矮 → 兜底，避免 MaxHeight 为 0/负值
			panelH = 300 * uiScale
		}
		// 不设 Width → 框架按内容自适应（fitWidth），面板贴合文字宽度，右侧不留空
		panel := ui.Box(c).Absolute().Right(12*uiScale).Top(panelTop).MaxWidth(panelW).MaxHeight(panelH).Radius(16 * uiScale).PaddingX(10 * uiScale).PaddingY(10 * uiScale).Background(clrCard)
		panel.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Shadow(r, 16, 0, 12, 40, 0, ui.RGBA(0, 0, 0, 0.45))
		})
		panel.Children(func() {
			ui.Column(c).Gap(4 * uiScale).Children(func() {
				items := []struct {
					tab   string
					label string
					icon  *ui.SVG
				}{
					{"queue", "队列", icList},
					{"fav", "歌单", icFav},
					{"search", "来源", icSearch},
					{"stats", "统计", icFav},
					{"cast", "目标", icCast},
					{"settings", "设置", icSet},
				}
				for _, it := range items {
					it := it
					a.navPanelRow(c, it.icon, it.label, a.activeTab == it.tab, func() {
						a.setTab(it.tab)
						a.navOpen = false
					})
				}
			})
		})
	})
}

// navPanelRow 侧滑面板里的一个导航项。
func (a *app) navPanelRow(c *ui.Context, s *ui.SVG, label string, active bool, onPick func()) {
	box := ui.Box(c).Fill().Height(42 * uiScale).Radius(10 * uiScale).PaddingX(12 * uiScale).Justify(ui.Center)
	if active {
		box.Background(clrField)
	} else if box.Hovered() {
		box.Background(clrField)
	}
	box.Children(func() {
		ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(12 * uiScale).Children(func() {
			col := clrMuted
			if active {
				col = clrPink
			}
			icon(c, s, 18, col)
			ui.Text(c, label).FontSize(14 * uiScale).TextColor(col)
		})
	})
	if box.Clicked() {
		onPick()
	}
}

// navTab 导航项：当前高亮为紫渐变胶囊；hover 淡高亮。iconOnly 时只显示图标。
func navTab(c *ui.Context, s *ui.SVG, label string, active bool, iconOnly bool) ui.Element {
	box := ui.Box(c).Height(40 * uiScale).Radius(20 * uiScale).PaddingX(16 * uiScale).AlignItems(ui.Center).Justify(ui.Center)
	if iconOnly {
		box = ui.Box(c).Size(40, 40).Radius(20 * uiScale).AlignItems(ui.Center).Justify(ui.Center)
	}
	if active {
		box.Draw(func(p *ui.Painter, r ui.Rect) {
			p.FillGradient(r, grad(), r.H/2)
		})
	} else {
		box.Draw(func(p *ui.Painter, r ui.Rect) {
			if box.Hovered() {
				p.Fill(r, ui.RGBA(0xff, 0xff, 255, 0.06), r.H/2)
			}
		})
	}
	box.Children(func() {
		col := clrMuted
		if active {
			col = ui.RGB(0xff, 0xff, 0xff)
		} else if box.Hovered() {
			col = clrText
		}
		ui.Row(c).Gap(8 * uiScale).AlignItems(ui.Center).Children(func() {
			icon(c, s, 18, col)
			if !iconOnly {
				ui.Text(c, label).FontSize(15 * uiScale).TextColor(col)
			}
		})
	})
	return box
}

// searchEmpty 空状态：居中大搜索框 + 热词标签 + 提示。
// searchBar 常驻胶囊搜索框（对齐主线 content_search.html：搜索结果页顶部也保留）。
