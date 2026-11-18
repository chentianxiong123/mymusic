//go:build native

package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"homecast/service/songlist"
)

// TestSonglistGridRenders 歌单列表页能渲染（有歌单 → 显示名称/首数；空 → 空态）
func TestSonglistGridRenders(t *testing.T) {
	a := &app{
		covers:       map[string]ui.ImageSource{},
		coverLoading: map[string]bool{},
		activeTab:    "songlists",
		songlists: []songlist.Songlist{
			{ID: 1, Name: "驾车歌单", Count: 2, Cover: ""},
			{ID: 2, Name: "空歌单", Count: 0, Cover: ""},
		},
	}
	tt := ui.NewTester(a.view, 900, 700)
	if !tt.HasText("歌单") {
		t.Errorf("缺标题「歌单」，texts=%q", tt.Texts())
	}
	if !tt.HasText("驾车歌单") {
		t.Errorf("缺歌单名「驾车歌单」，texts=%q", tt.Texts())
	}
	if !tt.HasText("2 首") {
		t.Errorf("缺首数「2 首」，texts=%q", tt.Texts())
	}
	if !tt.HasText("新建歌单") {
		t.Errorf("缺「新建歌单」按钮，texts=%q", tt.Texts())
	}
}

// TestSonglistEmptyRenders 空歌单页显示空态提示
func TestSonglistEmptyRenders(t *testing.T) {
	a := &app{
		covers:       map[string]ui.ImageSource{},
		coverLoading: map[string]bool{},
		activeTab:    "songlists",
		songlists:    nil,
	}
	tt := ui.NewTester(a.view, 900, 700)
	if !tt.HasText("还没有歌单") {
		t.Errorf("空态缺「还没有歌单」，texts=%q", tt.Texts())
	}
}

// TestSonglistDetailRenders 歌单详情页能渲染（歌曲行 + 首数 + 播放全部）
func TestSonglistDetailRenders(t *testing.T) {
	a := &app{
		covers:       map[string]ui.ImageSource{},
		coverLoading: map[string]bool{},
		activeTab:    "songlists",
		songlistOpen: 1,
		songlistCur:  songlist.Songlist{ID: 1, Name: "驾车歌单", Count: 1},
		songlistItems: []songlist.Item{
			{BVID: "BV1", Title: "七里香", Artist: "周杰伦", Cover: "", Duration: "4:30", DurationSec: 270},
		},
	}
	tt := ui.NewTester(a.view, 900, 700)
	if !tt.HasText("驾车歌单") {
		t.Errorf("缺歌单名，texts=%q", tt.Texts())
	}
	if !tt.HasText("七里香") {
		t.Errorf("缺歌曲名，texts=%q", tt.Texts())
	}
	if !tt.HasText("播放全部") {
		t.Errorf("缺「播放全部」，texts=%q", tt.Texts())
	}
	if !tt.HasText("周杰伦") {
		t.Errorf("缺歌手名，texts=%q", tt.Texts())
	}
}

// TestTitleToSongMygo 对齐主线 hcTitle：五种括号《》（）【】「」[] 提取 + 多个用「 · 」拼接。
func TestTitleToSongMygo(t *testing.T) {
	cases := []struct{ in, want string }{
		{"《七里香》周杰伦", "七里香"},
		{"【MV】（高清）[1080P]", "MV · 高清 · 1080P"}, // 三种括号全提取（对齐主线 hcTitle）
		{"「现场」七里香", "现场"},
		{"周杰伦 (Jay) 七里香", "周杰伦 (Jay) 七里香"}, // 半角 () 主线正则不匹配 → 返回原题
		{"纯标题无括号", "纯标题无括号"},
	}
	for _, c := range cases {
		if got := titleToSongMygo(c.in); got != c.want {
			t.Errorf("titleToSongMygo(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
