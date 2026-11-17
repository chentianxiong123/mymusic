// Package songlist 歌单域：用户自建歌单（多份、可命名）+ 每份歌单里的歌。
//
// 与 playlist 域（= 播放队列）的区别：
//   - playlist = 当前要播什么（临时，随播放推进，用户不直接分类管理）
//   - songlist = 用户分类存的东西（持久，可命名、可多份、可增删歌）
//
// 全链路：入口 internal/api/songlist.go · hx/songlist.go；
// 前端 content_songlists.html + songlist.js；表：songlists（PlaylistSchema 本包）
package songlist

import (
	"database/sql"
	"errors"
	"strings"
	"sync"
	"time"
)

// Songlist 歌单条目（含歌曲数 + 封面；对齐前端 Songlist）
type Songlist struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Count     int    `json:"count"` // 歌单内歌曲数（List/Get 时算）
	Cover     string `json:"cover"` // 封面：取最新一首的封面（卡片展示用；空歌单为空）
	CreatedAt int64  `json:"created_at"`
}

// Item 歌单内的一首（对齐前端 MusicItem 字段集）
type Item struct {
	BVID        string `json:"bvid"`
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	Cover       string `json:"cover"`
	Duration    string `json:"duration"`
	DurationSec int    `json:"duration_sec"`
	AddedAt     int64  `json:"added_at"`
}

// SonglistTable 歌单表名
const SonglistTable = "songlists"

// SonglistSchema 歌单表定义（表结构归业务域；建表执行在 store.Migrate 统一跑）
const SonglistSchema = `CREATE TABLE IF NOT EXISTS songlists (
	id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, created_at INTEGER DEFAULT 0)`

// SonglistItemTable 歌单条目表名
const SonglistItemTable = "songlist_items"

// SonglistItemSchema 歌单条目表定义。
// 主键 (songlist_id, bvid)：同歌单同歌只存一份（去重语义落在 DB，不靠应用层查）
const SonglistItemSchema = `CREATE TABLE IF NOT EXISTS songlist_items (
	songlist_id INTEGER NOT NULL, bvid TEXT NOT NULL,
	title TEXT, artist TEXT, cover TEXT,
	duration TEXT, duration_sec INTEGER,
	added_at INTEGER DEFAULT 0,
	PRIMARY KEY (songlist_id, bvid))`

// ErrNotFound 歌单不存在
var ErrNotFound = errors.New("歌单不存在")

// ErrEmptyName 歌单名为空
var ErrEmptyName = errors.New("歌单名不能为空")

// SonglistService 歌单（SQLite 持久化；增量 SQL，非全表重写）。
//
// 为什么不像 fav/playlist 那样「load 全表 → 改内存 → save 全表重写」：
// 歌单是两张表 + 可能几十份歌单，每次改一首就全库重写 O(n) 太重；
// 直接 INSERT/DELETE 增量操作更轻，且 ORDER BY added_at 自然给出排序。
type SonglistService struct {
	mu sync.Mutex
	db *sql.DB
}

func NewSonglistService(db *sql.DB) *SonglistService {
	return &SonglistService{db: db}
}

// List 全部歌单（新的在前）+ 每份歌曲数。
// 空结果返回空切片（非 nil）：JSON 序列化成 [] 而不是 null，前端 .length/.map 不用判空。
func (s *SonglistService) List() []Songlist {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`
		SELECT g.id, g.name, g.created_at, COUNT(i.bvid),
		       COALESCE((SELECT cover FROM songlist_items WHERE songlist_id=g.id ORDER BY added_at DESC, rowid DESC LIMIT 1), '')
		FROM songlists g LEFT JOIN songlist_items i ON i.songlist_id = g.id
		GROUP BY g.id ORDER BY g.id DESC`)
	// COALESCE：空歌单时子查询返回 NULL → Scan 到 string 报错 → 整行被丢弃（空歌单不显示的 bug）
	if err != nil {
		return []Songlist{}
	}
	defer rows.Close()
	out := []Songlist{}
	for rows.Next() {
		var g Songlist
		if rows.Scan(&g.ID, &g.Name, &g.CreatedAt, &g.Count, &g.Cover) == nil {
			out = append(out, g)
		}
	}
	return out
}

// Create 新建歌单（name 去空白后为空 → ErrEmptyName）
func (s *SonglistService) Create(name string) (Songlist, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Songlist{}, ErrEmptyName
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().Unix()
	res, err := s.db.Exec(`INSERT INTO songlists (name, created_at) VALUES (?,?)`, name, now)
	if err != nil {
		return Songlist{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Songlist{}, err
	}
	return Songlist{ID: id, Name: name, Count: 0, CreatedAt: now}, nil
}

// Rename 重命名（歌单不存在 → ErrNotFound；name 空 → ErrEmptyName）
func (s *SonglistService) Rename(id int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrEmptyName
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`UPDATE songlists SET name=? WHERE id=?`, name, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete 删除歌单（级联删条目；歌单不存在 → ErrNotFound）
func (s *SonglistService) Delete(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`DELETE FROM songlists WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	_, _ = s.db.Exec(`DELETE FROM songlist_items WHERE songlist_id=?`, id)
	return nil
}

// Get 歌单 + 歌单内歌曲（新的在前）；歌单不存在 → ErrNotFound
func (s *SonglistService) Get(id int64) (Songlist, []Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var g Songlist
	err := s.db.QueryRow(`SELECT id, name, created_at FROM songlists WHERE id=?`, id).
		Scan(&g.ID, &g.Name, &g.CreatedAt)
	if err == sql.ErrNoRows {
		return Songlist{}, nil, ErrNotFound
	}
	if err != nil {
		return Songlist{}, nil, err
	}
	items, err := s.queryItems(id)
	if err != nil {
		return Songlist{}, nil, err
	}
	g.Count = len(items)
	return g, items, nil
}

// queryItems 歌单内歌曲（调用方持锁）
func (s *SonglistService) queryItems(id int64) ([]Item, error) {
	rows, err := s.db.Query(`
		SELECT bvid, title, artist, cover, duration, duration_sec, added_at
		FROM songlist_items WHERE songlist_id=? ORDER BY added_at DESC, rowid DESC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		var it Item
		if rows.Scan(&it.BVID, &it.Title, &it.Artist, &it.Cover, &it.Duration, &it.DurationSec, &it.AddedAt) == nil {
			out = append(out, it)
		}
	}
	return out, nil
}

// AddSong 往歌单加一首（去重：已存在则不重复，返回 added=false）。
//
// 调用方（UI）通常已有完整条目信息 → 零网络请求；只有调用方只给 bvid（CLI/脚本）
// 时才需要自己先 GetVideoInfo 补全，本层不碰网络（保持域包纯粹、可离线测试）。
func (s *SonglistService) AddSong(id int64, it Item) (added bool, err error) {
	if it.BVID == "" {
		return false, errors.New("bvid required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// 歌单必须存在
	var n int
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM songlists WHERE id=?`, id).Scan(&n); err != nil {
		return false, err
	}
	if n == 0 {
		return false, ErrNotFound
	}
	if it.AddedAt == 0 {
		it.AddedAt = time.Now().Unix()
	}
	// INSERT OR IGNORE：主键 (songlist_id,bvid) 天然去重
	res, err := s.db.Exec(`
		INSERT OR IGNORE INTO songlist_items
		(songlist_id, bvid, title, artist, cover, duration, duration_sec, added_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		id, it.BVID, it.Title, it.Artist, it.Cover, it.Duration, it.DurationSec, it.AddedAt)
	if err != nil {
		return false, err
	}
	n2, _ := res.RowsAffected()
	return n2 > 0, nil
}

// RemoveSong 从歌单移除一首（不存在返回 false，不炸）
func (s *SonglistService) RemoveSong(id int64, bvid string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`DELETE FROM songlist_items WHERE songlist_id=? AND bvid=?`, id, bvid)
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n > 0
}

// Clear 清空歌单内歌曲（歌单保留）
func (s *SonglistService) Clear(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.db.Exec(`DELETE FROM songlist_items WHERE songlist_id=?`, id)
}
