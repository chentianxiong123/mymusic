//go:build native

package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestDoubleClickSimulate 模拟双击：Box + PassThrough 子元素，验证 DoubleClicked 是否触发。
// 目的：确认 songRow 的 PassThrough + DoubleClicked 机制是否可靠（双击不放的根因排查）。
func TestDoubleClickSimulate(t *testing.T) {
	var played int
	view := func(c *ui.Context) {
		ui.Row(c).Padding(20).Children(func() {
			box := ui.Box(c).Width(300).Height(50).Padding(10)
			box.Children(func() {
				ui.Row(c).Children(func() {
					// 模拟封面（PassThrough）
					ui.Box(c).Size(40, 40).Background(ui.RGB(0x22, 0x22, 0x22)).PassThrough()
					// 模拟标题（PassThrough）
					ui.Column(c).Grow(1).PassThrough().Children(func() {
						ui.Text(c, "这是一个很长很长很长很长很长的标题用于测试双击").MaxLines(1).PassThrough()
					})
				})
			})
			// 双击 box → 播放
			if box.DoubleClicked() {
				played++
			}
		})
	}

	tt := ui.NewTester(view, 600, 400)
	tt.Frame()

	// 找到标题元素中心，模拟双击（两次快速点击同点，headless ClickCounter 应判定 Clicks=2）
	r, ok := tt.Find("这是一个很长很长很长很长很长的标题用于测试双击")
	if !ok {
		t.Fatal("未找到标题元素")
	}
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	tt.ClickAt(cx, cy) // 第一次
	tt.Frame()
	tt.ClickAt(cx, cy) // 第二次（快速，应为双击）
	tt.Frame()

	t.Logf("双击触发播放次数: %d", played)
	if played == 0 {
		t.Errorf("双击未触发播放（bug 复现）")
	}
}
