//go:build live

// 联网验证（仅 -tags live 时跑）：demo JS 插件真实搜索/取流/歌词。
// 跑：GOFLAGS="-mod=mod -tags 'native live'" go test ./plugin/ -run TestLive -v
package plugin

import (
	"path/filepath"
	"testing"
)

func TestLiveDemoSearch(t *testing.T) {
	if _, err := LoadFromDir(filepath.Join("..", "plugins")); err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	src := Sources()["demo"]
	if src == nil {
		t.Fatal("demo source 未注册")
	}
	songs, total, err := src.Search("周杰伦", 1, 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if total == 0 || len(songs) == 0 {
		t.Fatal("搜索无结果")
	}
	t.Logf("✅ 搜索 total=%d 首曲=%s - %s (id=%s)", total, songs[0].Title, songs[0].Artist, songs[0].ID)

	st, err := src.GetStream(songs[0].ID, 1)
	if err != nil {
		t.Fatalf("取流失败: %v", err)
	}
	t.Logf("✅ 取流 url=%s quality=%d", st.URL, st.Quality)

	lines, err := src.GetLyric(songs[0].ID, nil, 0)
	if err != nil {
		t.Fatalf("歌词失败: %v", err)
	}
	first := ""
	if len(lines) > 0 {
		first = lines[0].Text
	}
	t.Logf("✅ 歌词 %d 行 首句=%q", len(lines), first)
}
