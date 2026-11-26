//go:build native

// stats_page_test.go —— 统计页/榜单气泡：固定高不爆字体 + 内容自适应居中。
package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestStatRowCenteredAndFixed(t *testing.T) {
	short := "1. 晴天 — 周杰伦 · 12 次"
	long := "1. 这是一个非常非常非常非常非常非常非常非常非常非常非常非常非常非常长的歌曲名字测试溢出 — 一个很长的歌手名字 — 123 次"
	view := func(c *ui.Context) {
		ui.Column(c).Gap(4).Children(func() {
			statRow(c, 350, "", short, "", false, nil)
			statRow(c, 350, "", long, "", false, nil)
		})
	}
	for _, w := range []int{1200, 500} {
		tt := ui.NewTester(view, w, 800)
		tt.Frame()
		for _, label := range []string{short, long} {
			r, ok := tt.Find(label)
			if !ok {
				t.Fatalf("未找到 %q", label)
			}
			centerX := r.X + r.W/2
			want := float32(w) / 2
			if centerX != want {
				t.Errorf("win=%d %q 未居中: centerX=%.0f want %.0f", w, label[:6], centerX, want)
			}
			if r.H > 30 {
				t.Errorf("win=%d %q 文字高度爆了: H=%.0f", w, label[:6], r.H)
			}
		}
	}
}
