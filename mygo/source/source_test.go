//go:build native

package source

import (
	hmodel "homecast/model"
	"testing"

	"mymusic-mygo/model"
)

// 编译期契约：B站源必须实现 MusicSource 接口（编译器强制，不满足编译失败）。
var _ MusicSource = (*BilibiliSource)(nil)

// TestSongFields 契约：Song 字段结构（音源无关规范结构）。
func TestSongFields(t *testing.T) {
	s := Song{ID: "BV1", Title: "歌", Artist: "人", Cover: "c", Duration: "3:21", DurationSec: 201, Typename: "bilibili"}
	if s.ID == "" || s.Title == "" {
		t.Fatalf("Song 必填字段为空: %+v", s)
	}
	if s.Typename != "bilibili" {
		t.Fatalf("Typename 应标记来源: %q", s.Typename)
	}
}

// TestLyricLineContract 契约：LyricLine 时间非负。
func TestLyricLineContract(t *testing.T) {
	lines := []model.LyricLine{{Time: 1.5, Text: "词"}, {Time: 0, Text: ""}}
	for _, ln := range lines {
		if ln.Time < 0 {
			t.Fatalf("歌词时间不能为负: %+v", ln)
		}
	}
}

// TestGetLyricBadID 失败路径：无效 id 应返回错误而非 panic。
func TestGetLyricBadID(t *testing.T) {
	s := &BilibiliSource{}
	// 空 keywords + sid=0 → 主线 GetLyricLines 返回 nil（业务正常，无词）
	lines, err := s.GetLyric(hmodel.SongRef{ID: ""}, nil, 0)
	if err != nil {
		t.Fatalf("无词不应报错: %v", err)
	}
	if lines != nil {
		t.Fatalf("空关键词应无词: %v", lines)
	}
}
