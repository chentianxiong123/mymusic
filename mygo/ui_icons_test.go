//go:build native

// ui_icons_test.go —— "词"字按钮与 SVG 图标按钮尺寸一致（都随 uiScale 缩放）。
package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestLyricToggleMatchesIconButton(t *testing.T) {
	// 回归：之前 lyricToggleButton 的 Box.Size 没乘 uiScale，
	// 而 iconButton2 乘了 → HiDPI（uiScale=1.5）下词按钮 24px、图标按钮 36px，视觉不齐。
	const size = float32(24)
	if uiScale <= 1 {
		t.Skipf("uiScale=%.1f，非 HiDPI 场景跳过（修复前后无差异）", uiScale)
	}
	view := func(c *ui.Context) {
		lyricToggleButton(c, size, true)
	}
	tt := ui.NewTester(view, 100, 100)
	tt.Frame()
	r, ok := tt.Find("词")
	if !ok {
		t.Fatal("未找到词字")
	}
	// 字高应接近按钮实际尺寸 size*uiScale（跟撑满按钮的 SVG 图标齐平）
	want := size * uiScale
	if r.H > want+2 || r.H < want-6 {
		t.Errorf("词字高 %.0f 与按钮实际尺寸 %.0f 不齐平", r.H, want)
	}
}
