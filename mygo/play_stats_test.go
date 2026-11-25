//go:build native

// play_stats_test.go —— 播放统计核心逻辑测试（不碰网络/窗口）。
package main

import (
	"database/sql"
	"testing"

	"homecast/service/conv"
)

func testStatsApp(t *testing.T) *app {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT)"); err != nil {
		t.Fatal(err)
	}
	return &app{db: db}
}

func TestPlayStatsRecord(t *testing.T) {
	a := testStatsApp(t)
	a.statsPlay("netease", "123", "晴天", "周杰伦", "")
	a.statsPlay("netease", "123", "晴天", "周杰伦", "")
	a.statsPlay("kugou", "abc", "夜曲", "周杰伦", "")

	s := a.loadPlayStats()
	if len(s.Songs) != 2 {
		t.Fatalf("want 2 songs, got %d", len(s.Songs))
	}
	if s.Songs[0].Count != 2 {
		t.Errorf("晴天 count want 2, got %d", s.Songs[0].Count)
	}
	if s.Songs[0].Source != "netease" || s.Songs[1].Source != "kugou" {
		t.Errorf("source 错: %+v", s.Songs)
	}
}

func TestPlayStatsAddSec(t *testing.T) {
	a := testStatsApp(t)
	a.statsPlay("bilibili", "BV1", "测试", "作者", "")
	a.statsAddSec("bilibili", "BV1", 30)
	a.statsAddSec("bilibili", "BV1", 30)
	a.statsAddSec("bilibili", "BVnone", 30) // 不存在的歌不panic
	s := a.loadPlayStats()
	if s.Songs[0].TotalSec != 60 {
		t.Errorf("totalSec want 60, got %f", s.Songs[0].TotalSec)
	}
}

func TestTopSongsAndArtists(t *testing.T) {
	a := testStatsApp(t)
	a.statsPlay("netease", "1", "晴天", "周杰伦", "")
	a.statsPlay("netease", "1", "晴天", "周杰伦", "")
	a.statsPlay("netease", "2", "夜曲", "周杰伦", "")
	a.statsPlay("kugou", "3", "其它", "别人", "")
	top := a.topSongs(2)
	if len(top) != 2 || top[0].Title != "晴天" {
		t.Errorf("topSongs 错: %+v", top)
	}
	artists := a.topArtists(2)
	if len(artists) != 2 || artists[0].Name != "周杰伦" || artists[0].Count != 3 {
		t.Errorf("topArtists 错: %+v", artists)
	}
}

func TestLyricSongBinding(t *testing.T) {
	a := testStatsApp(t)
	// 渠道级绑定
	_ = conv.SaveSetting(a.db, lyricBindPrefix+"netease", "netease")
	if got := a.lyricBoundFor("netease"); got != "netease" {
		t.Errorf("渠道绑定: %s", got)
	}
	// 无歌级绑定 → 回落渠道级
	if got := a.lyricBoundForSong("netease", "123"); got != "netease" {
		t.Errorf("歌级回落: %s", got)
	}
	// 歌级绑定 → 覆盖
	a.setLyricBoundForSong("netease", "123", "qqmusic")
	if got := a.lyricBoundForSong("netease", "123"); got != "qqmusic" {
		t.Errorf("歌级覆盖: %s", got)
	}
	if v, ok := a.songLyricBinding("netease", "123"); !ok || v != "qqmusic" {
		t.Errorf("songLyricBinding: %s %v", v, ok)
	}
	// 清除 → 回落
	a.clearLyricBoundForSong("netease", "123")
	if got := a.lyricBoundForSong("netease", "123"); got != "netease" {
		t.Errorf("清除后回落: %s", got)
	}
}
