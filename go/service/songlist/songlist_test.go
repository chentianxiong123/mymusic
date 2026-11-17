package songlist

import (
	"path/filepath"
	"testing"

	"homecast/store"
)

func newTestSonglist(t *testing.T) *SonglistService {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(db, []string{SonglistSchema, SonglistItemSchema}, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewSonglistService(db)
}

func item(bvid, title string) Item {
	return Item{BVID: bvid, Title: title, Artist: "a", Cover: "c", Duration: "3:00", DurationSec: 180}
}

// TestSonglistCreateEmptyName 空名拒绝（前后空白也拒绝）
func TestSonglistCreateEmptyName(t *testing.T) {
	s := newTestSonglist(t)
	for _, name := range []string{"", "   ", "\t"} {
		if _, err := s.Create(name); err != ErrEmptyName {
			t.Errorf("Create(%q) err=%v, want ErrEmptyName", name, err)
		}
	}
	if got := s.List(); len(got) != 0 {
		t.Errorf("空名不应建出歌单，实际 %d 份", len(got))
	}
}

// TestSonglistLifecycle 建 → 加歌 → 详情 → 改名 → 删（级联条目）
func TestSonglistLifecycle(t *testing.T) {
	s := newTestSonglist(t)

	g, err := s.Create("  我的歌单  ")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if g.Name != "我的歌单" { // 前后空白被裁掉
		t.Errorf("Name=%q, want 我的歌单（裁剪空白）", g.Name)
	}
	if g.Count != 0 {
		t.Errorf("新歌单 Count=%d, want 0", g.Count)
	}

	// 加两首（倒序加入，验证「新的在前」）
	if added, err := s.AddSong(g.ID, item("BV1", "一")); err != nil || !added {
		t.Fatalf("AddSong BV1: added=%v err=%v", added, err)
	}
	if added, err := s.AddSong(g.ID, item("BV2", "二")); err != nil || !added {
		t.Fatalf("AddSong BV2: added=%v err=%v", added, err)
	}

	got, items, err := s.Get(g.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Count != 2 || len(items) != 2 {
		t.Fatalf("Count=%d items=%d, want 2/2", got.Count, len(items))
	}
	if items[0].BVID != "BV2" {
		t.Errorf("items[0]=%s, want BV2（新的在前）", items[0].BVID)
	}

	// 改名
	if err := s.Rename(g.ID, "新名字"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	got, _, _ = s.Get(g.ID)
	if got.Name != "新名字" {
		t.Errorf("改名后 Name=%q, want 新名字", got.Name)
	}

	// 删除 → 条目级联清空
	if err := s.Delete(g.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, _, err := s.Get(g.ID); err != ErrNotFound {
		t.Errorf("删除后 Get err=%v, want ErrNotFound", err)
	}
	var left int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM songlist_items`).Scan(&left); err != nil {
		t.Fatalf("count items: %v", err)
	}
	if left != 0 {
		t.Errorf("级联删除后残留 %d 条 item", left)
	}
}

// TestSonglistDedup 同歌单加同一首：不重复，added=false
func TestSonglistDedup(t *testing.T) {
	s := newTestSonglist(t)
	g, _ := s.Create("g")
	s.AddSong(g.ID, item("BV1", "一"))
	added, err := s.AddSong(g.ID, item("BV1", "一"))
	if err != nil {
		t.Fatalf("重复 AddSong: %v", err)
	}
	if added {
		t.Errorf("重复加歌 added=true, want false")
	}
	if _, items, _ := s.Get(g.ID); len(items) != 1 {
		t.Errorf("去重后 items=%d, want 1", len(items))
	}
}

// TestSonglistRemoveClear 移除单首 + 清空（歌单保留）
func TestSonglistRemoveClear(t *testing.T) {
	s := newTestSonglist(t)
	g, _ := s.Create("g")
	s.AddSong(g.ID, item("BV1", "一"))
	s.AddSong(g.ID, item("BV2", "二"))

	if !s.RemoveSong(g.ID, "BV1") {
		t.Errorf("RemoveSong 存在的歌返回 false")
	}
	if s.RemoveSong(g.ID, "BV999") {
		t.Errorf("RemoveSong 不存在的歌返回 true")
	}
	if _, items, _ := s.Get(g.ID); len(items) != 1 || items[0].BVID != "BV2" {
		t.Errorf("移除后 = %+v, want 只剩 BV2", items)
	}

	s.Clear(g.ID)
	if _, items, _ := s.Get(g.ID); len(items) != 0 {
		t.Errorf("清空后 items=%d, want 0", len(items))
	}
	// 清空不删歌单本身
	if _, _, err := s.Get(g.ID); err != nil {
		t.Errorf("清空后歌单应仍在, err=%v", err)
	}
}

// TestSonglistNotFound 对不存在的歌单操作：加/改名/删都返回 ErrNotFound，不 panic
func TestSonglistNotFound(t *testing.T) {
	s := newTestSonglist(t)
	if _, err := s.AddSong(999, item("BV1", "一")); err != ErrNotFound {
		t.Errorf("AddSong 不存在歌单 err=%v, want ErrNotFound", err)
	}
	if err := s.Rename(999, "x"); err != ErrNotFound {
		t.Errorf("Rename 不存在歌单 err=%v, want ErrNotFound", err)
	}
	if err := s.Delete(999); err != ErrNotFound {
		t.Errorf("Delete 不存在歌单 err=%v, want ErrNotFound", err)
	}
}

// TestSonglistPersist 落盘：重建 service（同一 db 文件）数据仍在
func TestSonglistPersist(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, []string{SonglistSchema, SonglistItemSchema}, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	s1 := NewSonglistService(db)
	g, _ := s1.Create("落盘歌单")
	s1.AddSong(g.ID, item("BV9", "九"))
	db.Close()

	db2, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer db2.Close()
	s2 := NewSonglistService(db2)
	list := s2.List()
	if len(list) != 1 || list[0].Name != "落盘歌单" || list[0].Count != 1 {
		t.Errorf("重建后 List = %+v, want 落盘歌单/1 首", list)
	}
	if _, items, _ := s2.Get(g.ID); len(items) != 1 || items[0].BVID != "BV9" {
		t.Errorf("重建后 items = %+v, want BV9", items)
	}
}

// TestSonglistEmptyCover 空歌单必须在 List 里出现（cover 为 NULL 时 Scan 不炸）
//
// 回归：子查询 (SELECT cover ...) 对空歌单返回 NULL → Scan 到 string 报错 → 整行被丢弃
// → 空歌单不显示（曾漏掉，COALESCE 修复）
func TestSonglistEmptyCover(t *testing.T) {
	s := newTestSonglist(t)
	if _, err := s.Create("空歌单"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	list := s.List()
	if len(list) != 1 {
		t.Fatalf("空歌单未出现在 List，len=%d（cover NULL Scan 报错 bug）", len(list))
	}
	if list[0].Name != "空歌单" || list[0].Count != 0 || list[0].Cover != "" {
		t.Errorf("空歌单字段错: %+v", list[0])
	}
}

// TestSonglistMulti 多歌单互不干扰
func TestSonglistMulti(t *testing.T) {
	s := newTestSonglist(t)
	g1, _ := s.Create("甲")
	g2, _ := s.Create("乙")
	s.AddSong(g1.ID, item("BV1", "一"))
	s.AddSong(g2.ID, item("BV2", "二"))

	if _, items, _ := s.Get(g1.ID); len(items) != 1 || items[0].BVID != "BV1" {
		t.Errorf("歌单甲 = %+v, want BV1", items)
	}
	if _, items, _ := s.Get(g2.ID); len(items) != 1 || items[0].BVID != "BV2" {
		t.Errorf("歌单乙 = %+v, want BV2", items)
	}
	list := s.List()
	if len(list) != 2 {
		t.Fatalf("List = %d 份, want 2", len(list))
	}
	// 新的在前（乙先建 → id 大 → 在前）
	if list[0].Name != "乙" {
		t.Errorf("List[0]=%s, want 乙（新的在前）", list[0].Name)
	}
}
