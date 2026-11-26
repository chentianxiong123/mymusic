//go:build native

// pagination_test.go —— 分页基数一致性回归（搜索页 0 基 UI ↔ 1 基 API）。
package main

import (
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestPageList(t *testing.T) {
	cases := []struct {
		page, total int
		want        []int
	}{
		{0, 1, []int{0}},
		{0, 3, []int{0, 1, 2}},
		{1, 3, []int{0, 1, 2}},
		{2, 3, []int{0, 1, 2}},
		{0, 10, []int{0, 1, -1, 9}},
		{4, 10, []int{0, -1, 3, 4, 5, -1, 9}},
		{9, 10, []int{0, -1, 8, 9}},
	}
	for _, tc := range cases {
		got := pageList(tc.page, tc.total)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("pageList(%d,%d)=%v, want %v", tc.page, tc.total, got, tc.want)
		}
	}
}

// TestSearchPageBaseMapping 锁定 sourcePage（1 基 API）与 searchPages（0 基 UI）的转换。
func TestSearchPageBaseMapping(t *testing.T) {
	a := &app{}
	// 默认第 1 页
	if got := a.sourcePage("netease"); got != 1 {
		t.Fatalf("默认页: want 1, got %d", got)
	}
	// UI 点第 1 页按钮（p=0）→ searchPages[name]=0 → sourcePage 返回 1
	a.searchPages = map[string]int{"netease": 0}
	if got := a.sourcePage("netease"); got != 1 {
		t.Errorf("UI 第1页: want API page 1, got %d", got)
	}
	// UI 点第 2 页按钮（p=1）→ sourcePage 返回 2
	a.searchPages["netease"] = 1
	if got := a.sourcePage("netease"); got != 2 {
		t.Errorf("UI 第2页: want API page 2, got %d", got)
	}
}

// TestPagerButtonsClick 用 Tester 模拟点击页码按钮，验证 0 基回调正确。
func TestPagerButtonsClick(t *testing.T) {
	page := 0
	view := func(c *ui.Context) {
		a := &app{}
		a.pagerButtons(c, page, 3, func(p int) { page = p })
	}
	tt := ui.NewTester(view, 600, 200)
	tt.Frame()

	// 找页码"3"的按钮文本，点击 → 回调应收到 2（0 基），不是 3
	r, ok := tt.Find("3")
	if !ok {
		t.Fatal("未找到页码 3")
	}
	tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	tt.Frame()
	if page != 2 {
		t.Errorf("点第3页按钮: want 回调 2(0基), got %d", page)
	}
}
