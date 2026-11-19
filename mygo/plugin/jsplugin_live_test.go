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
	src := Sources()["netease"]
	if src == nil {
		t.Fatal("netease source 未注册")
	}
	songs, total, err := src.Search("周杰伦", 1, 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if total == 0 || len(songs) == 0 {
		t.Fatal("搜索无结果")
	}
	t.Logf("✅ 搜索 total=%d 首曲=%s - %s (id=%s)", total, songs[0].Title, songs[0].Artist, songs[0].ID)
	t.Logf("✅ 封面 cover=%q 专辑=%q VIP=%v", songs[0].Cover, songs[0].Album, songs[0].VIP)
	if songs[0].Cover == "" {
		t.Error("封面为空（搜索接口应给歌手图 img1v1Url 兜底）")
	}

	st, err := src.GetStream(songs[0].ID, 1)
	if err != nil {
		t.Fatalf("取流失败: %v", err)
	}
	t.Logf("✅ 取流 url=%s quality=%d", st.URL, st.Quality)

	// 歌词：demo 不声明 lyric 能力（网易云歌词走内置 netease 源），
	// GetLyric 返回 (nil, nil)。若未来 JS 音源自带歌词，声明 capability "lyric" 即可。
	lines, err := src.GetLyric(songs[0].ID, nil, 0)
	if err != nil {
		t.Fatalf("歌词失败: %v", err)
	}
	t.Logf("✅ netease 音源无 lyric 能力，歌词走内置 netease（GetLyric 返回 %d 行）", len(lines))
}
