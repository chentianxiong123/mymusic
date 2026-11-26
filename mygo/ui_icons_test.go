//go:build native

// ui_icons_test.go —— "词"字按钮与旁边 SVG 图标视觉齐平（字高 ≈ 按钮 size）。
package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestLyricToggleGlyphFits(t *testing.T) {
	const size = float32(24)
	view := func(c *ui.Context) {
		lyricToggleButton(c, size, true)
	}
	tt := ui.NewTester(view, 100, 100)
	tt.Frame()
	r, ok := tt.Find("词")
	if !ok {
		t.Fatal("未找到词字")
	}
	// 字高应 ≤ size（不溢出按钮），且接近 size（跟撑满按钮的 SVG 图标齐平）
	if r.H > size+1 || r.H < size-3 {
		t.Errorf("词字高 %.0f 与按钮 size %.0f 不齐平（应 ≈size 且不溢出）", r.H, size)
	}
}
