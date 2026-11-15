//go:build native

package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"homecast/bilibili"
)

func TestCardRenders(t *testing.T) {
	a := &app{covers: map[string]ui.ImageSource{}, coverLoading: map[string]bool{}}
	a.results = []bilibili.SearchItem{
		{Title: "测试歌名<em>x</em>", Author: "作者甲", Duration: "3:21", Pic: ""},
	}
	tt := ui.NewTester(a.view, 560, 800)
	if !tt.HasText("测试歌名x") {
		t.Fatalf("card not rendered, texts=%q", tt.Texts())
	}
}
