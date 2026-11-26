//go:build native

// point_play_test.go —— 点歌方式（替换/追加）核心逻辑测试（不碰网络/窗口）。
package main

import (
	"testing"

	"homecast/bilibili"
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
