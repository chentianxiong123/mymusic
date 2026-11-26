//go:build native

// search_present_test.go —— 呈现层过滤/排序（不污染数据接口）回归测试。
package main

import (
	"testing"

	"mymusic-mygo/source"
)

func TestPresentSongs(t *testing.T) {
	songs := []source.Song{
		{ID: "c", Title: "【4K】周杰伦《最伟大的作品》", Typename: "音乐", DurationSec: 240},
		{ID: "b", Title: "告白气球", Typename: "音乐", DurationSec: 180},
		{ID: "x", Title: "零基础教程", Typename: "音乐教学", DurationSec: 300}, // 非歌 → 滤掉
		{ID: "d", Title: "晴天", Typename: "音乐", DurationSec: 150},
		{ID: "z", Title: "无时长", Typename: "音乐", DurationSec: 0}, // 时长 0：IsSong 不判死，保留
	}
	out := presentSongs(songs)
	for _, s := range out {
		if s.ID == "x" {
			t.Fatal("非歌（音乐教学）未滤掉")
		}
	}
	// 排序：晴天(0/150) → 告白气球(0/180) → 无时长(0/0) → 【4K】(3/240)
	// 无时长 DurationSec=0 在升序里排最前，属现有 IsSong 语义（不强判）；这里只验核心顺序。
	if len(out) != 4 {
		t.Fatalf("want 4 条(滤掉教学), got %d", len(out))
	}
	if out[0].ID != "z" || out[1].ID != "d" || out[2].ID != "b" || out[3].ID != "c" {
		t.Errorf("排序错: got %v", func() []string {
			ids := []string{}
			for _, s := range out {
				ids = append(ids, s.ID)
			}
			return ids
		}())
	}
	// 原切片未被修改（呈现层不动数据）
	if songs[0].ID != "c" || songs[2].ID != "x" {
		t.Error("presentSongs 修改了原切片")
	}
}
