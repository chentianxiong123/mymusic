package main

import (
	"path/filepath"
	"testing"

	"homecast/service/songlist"
	"homecast/store"
)

// TestSonglistServiceInMygo mygo 侧歌单链路：建表（同 pb_bridge 的 schema 列表）→ 建歌单 → 加歌 → 播放全部转换
func TestSonglistServiceInMygo(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := store.Migrate(db, []string{
		songlist.SonglistSchema, songlist.SonglistItemSchema,
	}, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := songlist.NewSonglistService(db)

	// 建歌单
	g, err := svc.Create("驾车")
	if err != nil || g.ID == 0 {
		t.Fatalf("Create: %+v err=%v", g, err)
	}
	// 加两首
	svc.AddSong(g.ID, songlist.Item{BVID: "BV1", Title: "一", Artist: "A", Cover: "c1", Duration: "3:00", DurationSec: 180})
	svc.AddSong(g.ID, songlist.Item{BVID: "BV2", Title: "二", Artist: "B", Cover: "c2", Duration: "4:00", DurationSec: 240})

	// 列表
	list := svc.List()
	if len(list) != 1 || list[0].Count != 2 || list[0].Cover != "c2" {
		t.Errorf("List = %+v, want 驾车/2首/封面c2", list)
	}
	// 详情 + 转 SearchItem（playAll 入参）
	_, items, _ := svc.Get(g.ID)
	si := songlistToSearchItems(items)
	if len(si) != 2 || si[0].BVID != "BV2" || si[0].Author != "B" {
		t.Errorf("songlistToSearchItems = %+v, want 2 首新的在前", si)
	}
}
