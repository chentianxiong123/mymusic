//go:build native

// stats_page_test.go —— 统计页/榜单行不"爆字体"：固定高度 + 单行截断。
package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestStatRowFixedHeight(t *testing.T) {
	long := "这是一个非常非常非常非常非常非常非常非常非常非常非常非常非常非常长的歌曲名字测试溢出"
	view := func(c *ui.Context) {
		ui.Column(c).Gap(4).Children(func() {
			statRow(c, "1.", long+" — "+long, "123 次", false, nil)
		})
	}
	// 不同窗口宽度下，文字高度必须固定（单行），不随宽度变窄而 wrap 膨胀
	var heights []float32
	for _, w := range []int{1200, 800, 400} {
		tt := ui.NewTester(view, w, 800)
		tt.Frame()
		r, ok := tt.Find(long + " — " + long)
		if !ok {
			t.Fatal("未找到名称文字")
		}
		heights = append(heights, r.H)
	}
	if heights[0] != heights[1] || heights[1] != heights[2] {
		t.Errorf("名称文字高度随窗口变化（wrap 爆格子）: %v", heights)
	}
	if heights[0] > 30 {
		t.Errorf("单行文字高度过大: %.0f", heights[0])
	}
}
