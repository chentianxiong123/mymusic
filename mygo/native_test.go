//go:build native

package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"homecast/bilibili"
	"mymusic-mygo/source"
)

func TestCardRenders(t *testing.T) {
	a := &app{covers: map[string]ui.ImageSource{}, coverLoading: map[string]bool{}, searchPages: map[string]int{}}
	a.results = []bilibili.SearchItem{
		{Title: "测试歌名<em>x</em>", Author: "作者甲", Duration: "3:21", Pic: ""},
	}
	// 搜索结果按源分组渲染（插件分组）：测试设一个源分区
	a.resultsBySource = []srcResult{{
		name:  "bilibili",
		total: 1,
		songs: []source.Song{{ID: "BV1", Title: "测试歌名<em>x</em>", Artist: "作者甲", Duration: "3:21", Typename: "bilibili"}},
	}}
	tt := ui.NewTester(a.view, 560, 800)
	if !tt.HasText("测试歌名x") {
		t.Fatalf("card not rendered, texts=%q", tt.Texts())
	}
}
