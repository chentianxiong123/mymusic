//go:build native

package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// view 无窗口也能构建。测试只检查初始界面，不点搜索按钮（避免发真实网络）。
func TestViewSmoke(t *testing.T) {
	a := &app{}
	tt := ui.NewTester(a.view, 560, 640)
	if !tt.HasText("按风格找歌") {
		t.Errorf("missing genre hint, texts %q", tt.Texts())
	}
	if !tt.HasText("未在播放") {
		t.Errorf("missing player idle text, texts %q", tt.Texts())
	}
}
