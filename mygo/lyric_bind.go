//go:build native

// lyric_bind.go —— 音源插件 → 歌词源绑定（每个音源配一个取词插件）。
// 绑定存 settings 键 lyricbind/<sourceName>（值=歌词插件名）；
// 未绑定且音源自带歌词插件（同名）→ 用自带；否则默认 netease。
package main

import (
	"homecast/service/conv"
	"mymusic-mygo/plugin"

	"github.com/egoist/mygo/ui"
)

const lyricBindPrefix = "lyricbind/"

// lyricBoundFor 取某音源绑定的歌词插件名。
// 规则：用户配置 > 插件自带（同名 lyric provider）> 默认 netease。
func (a *app) lyricBoundFor(src string) string {
	if src == "" {
		src = "bilibili"
	}
	if a.db != nil {
		if v, ok := conv.LoadSetting(a.db, lyricBindPrefix+src); ok && v != "" {
			return v
		}
	}
	// 音源插件自带歌词（同名 provider 已注册）→ 用它
	if plugin.Lyric(src) != nil {
		return src
	}
	return "netease"
}

// setLyricBoundFor 设置音源的歌词绑定（持久化）。
func (a *app) setLyricBoundFor(src, lyricName string) {
	if src == "" || a.db == nil {
		return
	}
	_ = conv.SaveSetting(a.db, lyricBindPrefix+src, lyricName)
}

// lyricBindOptions 可选歌词插件名列表（启用 + 已注册），供下拉展示。
func (a *app) lyricBindOptions() []string {
	return plugin.LyricNames()
}

// lyricBindSection 音源插件详情页的「歌词源」配置区块。
// 显示当前绑定 + 选择入口：列表为已注册歌词插件（来自插件页歌词分页）。
func (a *app) lyricBindSection(c *ui.Context, src string) {
	bound := a.lyricBoundFor(src)
	// 下拉绑持久字段：切插件/首次进入时同步一次，之后不再覆盖用户展开态
	if a.lyricBindSel == "" || a.lyricBindOwn != src {
		a.lyricBindSel = bound
		a.lyricBindOwn = src
	}
	selfHas := plugin.Lyric(src) != nil
	label := bound
	if selfHas && bound == src {
		label = "插件自带"
	}
	ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).PaddingX(4 * uiScale).Children(func() {
		ui.Text(c, "歌词源").FontSize(14 * uiScale).Bold().TextColor(clrText)
		ui.Spacer(c).Grow(1)
		ui.Text(c, label).FontSize(12 * uiScale).TextColor(clrPink)
	})
	ui.Text(c, "当前音源的取词插件（插件自带歌词或在「插件 → 歌词」中启用并选择）").
		FontSize(11 * uiScale).TextColor(clrMuted).PaddingX(4 * uiScale)

	// 下拉选绑定（点击每项即保存并关闭弹层）
	names := a.lyricBindOptions()
	if len(names) == 0 {
		return
	}
	sel := ui.SelectBase(c, &a.lyricBindSel)
	sel.Trigger.Height(36 * uiScale).PaddingX(12 * uiScale).Draw(func(p *ui.Painter, r ui.Rect) {
		if sel.Trigger.Hovered() {
			p.FillRadii(r, [4]float32{8, 8, 8, 8}, clrCardHov)
		}
	}).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(6 * uiScale).Children(func() {
			ui.Text(c, "选择歌词源").FontSize(12 * uiScale).TextColor(clrText)
			ui.Text(c, "▾").FontSize(11 * uiScale).TextColor(clrMuted)
		})
	})
	sel.Popup(func(panel ui.Element) {
		panel.Padding(6 * uiScale).Background(clrCard).Radius(12 * uiScale).MaxHeight(320 * uiScale)
		ui.Scroll(c).Grow(1).Scrollbars(false).Children(func() {
			for _, n := range names {
				n := n
				item := sel.Item(n).Height(38 * uiScale).PaddingX(12 * uiScale)
				if n == a.lyricBindSel {
					item.Background(clrCardHov)
				}
				item.Children(func() {
					tag := ""
					if n == src && selfHas {
						tag = " · 插件自带"
					}
					ui.Text(c, n+tag).FontSize(12 * uiScale).TextColor(clrText)
				})
				if item.Clicked() {
					a.setLyricBoundFor(src, n)
					if a.current != nil {
						a.loadLyricAsync(a.current.bvid)
					}
				}
			}
		})
	})
}
