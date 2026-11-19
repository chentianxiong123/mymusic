package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLocalSource 本地文件源：扫目录/文件名匹配/取流 file:///无歌词。
func TestLocalSource(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "晴天 - 周杰伦.mp3"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "七里香.flac"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("x"), 0o644)
	s := newLocalSource(dir)

	songs, total, err := s.Search("晴天", 1, 20)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if total != 1 || len(songs) != 1 {
		t.Fatalf("total=%d len=%d 期望 1", total, len(songs))
	}
	if songs[0].Title != "晴天 - 周杰伦" {
		t.Fatalf("title=%q", songs[0].Title)
	}
	if songs[0].Typename != "local" {
		t.Fatalf("typename=%q 期望 local", songs[0].Typename)
	}

	// 全量（含 flac，排除 txt）。
	all, total, err := s.Search("", 1, 20)
	if err != nil || total != 2 || len(all) != 2 {
		t.Fatalf("全量 total=%d len=%d 期望 2", total, len(all))
	}

	// 取流 file://。
	st, err := s.GetStream(all[0].ID, 0)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if !strings.HasPrefix(st.URL, "file://") {
		t.Fatalf("URL=%q 期望 file:// 前缀", st.URL)
	}

	// 无歌词（nil，不报错）。
	ly, err := s.GetLyric(all[0].ID, nil, 0)
	if err != nil || len(ly) != 0 {
		t.Fatalf("lyric: %v %d 行，期望 nil/0", err, len(ly))
	}
}
