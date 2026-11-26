//go:build native

// log_detail.go —— 点日志行展开全文（响应式 dialogRect，适配不同尺寸）。
package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *app) logDetailDialog(c *ui.Context) {
	e := a.logDetail
	if e == nil {
		return
	}
	ui.Overlay(c, func() {
		lx, ly, pw, ph := a.dialogRect(480*uiScale, 320*uiScale)
		bg := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0)
		bg.Draw(func(p *ui.Painter, r ui.Rect) { p.Fill(r, ui.RGBA(0, 0, 0, 0.55), 0) })
		if bg.Clicked() {
			a.logDetail = nil
			return
		}
		box := ui.Box(c).Absolute().Left(lx).Top(ly).Width(pw).Height(ph).
			Radius(16 * uiScale).Background(clrCard).Padding(16 * uiScale)
		box.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Shadow(r, 24, 0, 14, 48, 0, ui.RGBA(0, 0, 0, 0.6))
			p.FillRadii(r, [4]float32{16, 16, 16, 16}, clrCard)
		})
		box.Children(func() {
			ui.Column(c).Fill().Gap(10 * uiScale).Children(func() {
				// 标题栏
				ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Children(func() {
					ui.Text(c, e.Level.String()).FontSize(13*uiScale).Bold().TextColor(logLevelColor(e.Level))
					ui.Text(c, "["+e.Category+"]").FontSize(12*uiScale).TextColor(clrPink)
					ui.Text(c, e.Time.Format("2006/01/02 15:04:05")).FontSize(11*uiScale).TextColor(clrMuted)
					ui.Spacer(c).Grow(1)
					if chip(c, "×").Clicked() {
						a.logDetail = nil
					}
				})
				// 全文（可滚动；TextInput 可选中复制）
				ui.Scroll(c).Grow(1).Scrollbars(false).Children(func() {
					msg := e.Message
					ui.TextInput(c, &msg).Grow(1).Fill().FontSize(12*uiScale).TextColor(clrText)
				})
				// 复制按钮
				if chip(c, "复制内容").Clicked() {
					mygo.Clipboard.WriteText(e.Message)
					a.status = "已复制日志内容"
				}
			})
		})
	})
}
