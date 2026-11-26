//go:build native

// ui_icons_test.go —— 图标按钮尺寸框架契约：所有 xxxButton(size) 的 size 是逻辑像素，
// 函数内部统一乘 uiScale（HiDPI 下不能有的放大、有的不放大）。
package main

import (
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestButtonsScaleConsistently(t *testing.T) {
	if uiScale <= 1 {
		t.Skipf("uiScale=%.1f，非 HiDPI 场景跳过", uiScale)
	}
	const logic = float32(24)
	want := logic * uiScale

	view := func(c *ui.Context) {
		ui.Row(c).Gap(4).Children(func() {
			iconButton2(c, icRepeat, logic, clrText)
			lyricToggleButton(c, logic, true)
			playIconButton(c, icPlay, logic, clrText)
			heartButton(c, false, logic, func() {})
		})
	}
	tt := ui.NewTester(view, 500, 100)
	tt.Frame()

	// SVG 图标撑满按钮 = want；"词"字高应与之接近
	r, ok := tt.Find("词")
	if !ok {
		t.Fatal("未找到词字")
	}
	if r.H > want+2 || r.H < want-8 {
		t.Errorf("词按钮未与图标按钮同尺度: 字高 %.0f, want ≈%.0f", r.H, want)
	}
}

// TestSizeContractNoRawSize 结构性防回归：按钮类函数里不能有裸 Size(size, size)。
func TestSizeContractNoRawSize(t *testing.T) {
	for _, file := range []string{"ui_icons.go", "ui_modern.go"} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "Size(size, size)") {
			t.Errorf("%s 存在裸 Size(size, size)（漏乘 uiScale）", file)
		}
	}
}
