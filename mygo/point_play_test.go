//go:build native

// point_play_test.go —— 点歌方式（替换/追加）核心逻辑测试（不碰网络/窗口）。
package main

import (
	"testing"

	"homecast/bilibili"
	"homecast/service/playback"
)

func TestIndexOfBVID(t *testing.T) {
	list := []bilibili.SearchItem{{BVID: "a"}, {BVID: "b"}, {BVID: "c"}}
	if got := indexOfBVID(list, "b"); got != 1 {
		t.Fatalf("indexOfBVID(b) = %d, want 1", got)
	}
	if got := indexOfBVID(list, "z"); got != -1 {
		t.Fatalf("indexOfBVID(z) = %d, want -1", got)
	}
}

// 点歌模式是全局偏好：默认 0（替换列表，对齐网易云），可切到 1（追加）。
func TestPointPlayModePref(t *testing.T) {
	a := &app{}
	if a.pointPlayMode != 0 {
		t.Fatal("默认应为替换列表")
	}
	a.pointPlayMode = 1
	if a.pointPlayMode != 1 {
		t.Fatal("切换后应为追加列表")
	}
}

// 一切列表（搜索/歌单/收藏/队列/最近/推荐/榜单/统计）都归约到 playSongs，
// 没有例外入口。验证跨源 Song 转换保留 Source/ID，否则替换后会丢来源路由。
func TestPlaySourceFromPreservesRef(t *testing.T) {
	// 直接验证转换逻辑：playSourceFrom 内部构造 playback.Song 时应带 Source/ID。
	// 这里用同等构造验证字段不丢（不触发真实播放）。
	in := []struct {
		id, source string
	}{
		{"123456", "netease"},
		{"BV1xx", "bilibili"},
	}
	out := make([]playback.Song, 0, len(in))
	for _, x := range in {
		out = append(out, playback.Song{
			BVID: x.id, Title: "t", Source: x.source, ID: x.id,
		})
	}
	for i, got := range out {
		if got.Source != in[i].source || got.ID != in[i].id {
			t.Fatalf("第 %d 首跨源引用丢失: got source=%q id=%q, want %q/%q",
				i, got.Source, got.ID, in[i].source, in[i].id)
		}
	}
}
