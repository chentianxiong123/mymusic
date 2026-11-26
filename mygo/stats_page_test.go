//go:build native

// stats_page_test.go —— 统计页/榜单行：文字垂直居中 + 铺满整行（不左右居中）+ 不爆字体。
package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// 注：Tester 里 MaxLines(1) 会把文字矩形撑成固定 27px 高，Y 偏移不代表视觉居中；
// 真正的垂直居中由 statRow 的 Box.Justify(ui.Center) 保证（诊断验证无 MaxLines 时 Y=10）。
func TestStatRowVerticalCenter(t *testing.T) {
	long := "这是一个非常非常非常非常非常非常非常非常非常非常非常非常非常非常长的歌曲名字测试溢出"
	view := func(c *ui.Context) {
		ui.Column(c).Gap(4).Children(func() {
			statRow(c, "1.", long, "123 次", false, nil)
			statRow(c, "2.", "晴天 — 周杰伦", "12 次", false, nil)
		})
	}
	for _, w := range []int{1200, 500} {
		tt := ui.NewTester(view, w, 800)
		tt.Frame()
		for _, txt := range []string{long, "晴天 — 周杰伦"} {
			r, ok := tt.Find(txt)
			if !ok {
				t.Fatalf("未找到 %q", txt)
			}
			// 铺满整行左对齐：名称 X 应靠左（<100），不是窗口中心
			if r.X > 100 {
				t.Errorf("win=%d %q 被左右居中了: X=%.0f", w, txt[:6], r.X)
			}
			if r.H > 30 {
				t.Errorf("win=%d %q 文字高度爆了: H=%.0f", w, txt[:6], r.H)
			}
		}
	}
}
