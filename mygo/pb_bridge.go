//go:build native

// pb_bridge：MyGo ↔ 主线 playback 域包的桥接层。
// - ensureDB：SQLite 持久化（主线 store 驱动 + playback 用的 settings 表）
// - pbLoop：订阅主线状态流（state 全量快照 / progress 进度）→ win.Update 渲染
// - progressReporter：MyGo 本地 audio 进度 → 上报 SSOT（等价 web timeupdate）
// - applyState / 转换：主线 State → MyGo UI 状态字段（UI 兼容，读 a.current 等不变）
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"homecast/service/cache"
	"homecast/service/conv"
	"homecast/service/fav"
	"homecast/service/lyric"
	"homecast/service/lyriccache"
	"homecast/service/playback"
	"homecast/store"

	"homecast/bilibili"
)

// ensureDB 打开 SQLite（主线 store 驱动）+ 建 playback 用的 settings 表。
func ensureDB() *sql.DB {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	dir = filepath.Join(dir, "mymusic-mygo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("[mygo] mkdir data dir: %v", err)
	}
	db, err := store.Open(filepath.Join(dir, "data.db"))
	if err != nil {
		log.Printf("[mygo] open db: %v", err)
		return nil
	}
	// 建业务表（对齐主线 server.go Migrate：favs/lyric/cache/lyriccache/settings）。
	// 否则 fav.Add 等写表失败 → 收藏/歌词缓存不持久化（表不存在）。
	if err := store.Migrate(db, []string{
		fav.FavSchema,
		lyric.LyricSchema,
		cache.CacheSchema,
		cache.SettingsSchema,
		lyriccache.CacheSchema,
	}, nil); err != nil {
		log.Printf("[mygo] migrate tables: %v", err)
	}
	return db
}

// pbLoop 订阅主线状态流：state=全量快照渲染，progress=进度更新。全部经 win.Update 回主线程。
func (a *app) pbLoop() {
	ch := a.pb.Subscribe()
	defer a.pb.Unsubscribe(ch)
	for msg := range ch {
		switch msg.Name {
		case "state":
			var st playback.State
			if json.Unmarshal([]byte(msg.Data), &st) == nil {
				a.win.Update(func() { a.applyState(st) })
			}
		case "progress":
			var pr struct {
				Position float64 `json:"position"`
				Duration float64 `json:"duration"`
				Playing  bool    `json:"playing"`
			}
			if json.Unmarshal([]byte(msg.Data), &pr) == nil {
				a.win.Update(func() {
					if pr.Duration > 0 {
						f := pr.Position / pr.Duration
						if f < 0 {
							f = 0
						}
						if f > 1 {
							f = 1
						}
						a.progress = f
					}
					a.posText = fmtDurMygo(pr.Position)
					a.durText = fmtDurMygo(pr.Duration)
					a.playing = pr.Playing
					a.win.Invalidate()
				})
			}
		}
	}
}

// progressReporter MyGo 本地 audio 进度 → 上报 SSOT（等价 web timeupdate）。
// 仅 source=web 接受（MyGo 默认 web 语义），与音箱模式隔离。
func (a *app) progressReporter() {
	t := time.NewTicker(500 * time.Millisecond)
	defer t.Stop()
	for range t.C {
		info := a.mygoOut.Position()
		if info.Dur > 0 {
			a.pb.SetProgress(info.Pos, info.Dur, info.Playing)
			a.posSec = info.Pos
		}
		// dock 封面 EQ 频谱相位（播放中跳动；对齐主线 d-eq CSS 动画）
		if info.Playing {
			a.eqPhase++
		} else {
			a.eqPhase = -1 // -1 = 不显示
		}
		a.win.Invalidate()
		// 桌面挂件歌词跟着动（挂件是独立窗口，需单独重绘）
		if a.lyricWin != nil {
			a.lyricWin.Invalidate()
		}
	}
}

// savePref 设置键值持久化（settings 表；对齐主线 localStorage hc:* 语义）。
func (a *app) savePref(key string, v int) {
	if a.db == nil {
		return
	}
	a.db.Exec(`INSERT INTO settings (key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=?`, key, fmt.Sprintf("%d", v), fmt.Sprintf("%d", v))
}

// loadPref 读取设置键值（缺省返回 def）。
func (a *app) loadPref(key string, def int) int {
	if a.db == nil {
		return def
	}
	v, err := conv.LoadLimit(a.db, key, def)
	if err != nil {
		return def
	}
	return v
}

// saveRecent 最近播放落库（JSON；对齐主线 localStorage hc:recent）。
func (a *app) saveRecent() {
	if a.db == nil {
		return
	}
	data, err := json.Marshal(a.recent)
	if err != nil {
		return
	}
	a.db.Exec(`INSERT INTO settings (key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=?`,
		"mygo_recent", string(data), string(data))
}

// restoreRecent 启动恢复最近播放。
func (a *app) restoreRecent() {
	if a.db == nil {
		return
	}
	var v string
	if err := a.db.QueryRow(`SELECT value FROM settings WHERE key=?`, "mygo_recent").Scan(&v); err != nil {
		return
	}
	var list []bilibili.SearchItem
	if json.Unmarshal([]byte(v), &list) == nil {
		a.recent = list
	}
}

// saveHistory 搜索历史落库（JSON；对齐主线 localStorage search-history）。
func (a *app) saveHistory() {
	if a.db == nil {
		return
	}
	data, err := json.Marshal(a.searchHistory)
	if err != nil {
		return
	}
	a.db.Exec(`INSERT INTO settings (key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=?`,
		"mygo_history", string(data), string(data))
}

// restoreHistory 启动恢复搜索历史。
func (a *app) restoreHistory() {
	if a.db == nil {
		return
	}
	var v string
	if err := a.db.QueryRow(`SELECT value FROM settings WHERE key=?`, "mygo_history").Scan(&v); err != nil {
		return
	}
	var list []string
	if json.Unmarshal([]byte(v), &list) == nil {
		a.searchHistory = list
	}
}

// savePrefFloat 浮点偏好（音量等）。
func (a *app) savePrefFloat(key string, v float64) {
	a.savePref(key, int(v*1000))
}

// loadPrefFloat 读取浮点偏好（v=千分比存储；缺省返回 0.8）。
func (a *app) loadPrefFloat(key string, def float64) float64 {
	n := a.loadPref(key, int(def*1000))
	return float64(n) / 1000.0
}

// restorePrefs 启动恢复本机偏好（主题/音质/缓存上限，对齐主线 settings.js 持久化）。
func (a *app) restorePrefs() {
	a.themeMode = a.loadPref("mygo_theme", 0)
	a.quality = a.loadPref("mygo_quality", 2)
	a.cacheLimit = a.loadPref("mygo_cachelimit", 0)
	a.loopMode = a.loadPref("mygo_loopmode", 0)
	a.lyricSize = a.loadPref("mygo_lyricsize", 2)
	a.volume = a.loadPrefFloat("mygo_volume", 0.8)
	if a.svc != nil && a.svc.speaker != nil && a.loopMode != 0 {
		a.pb.SetMode(intToMode(a.loopMode))
	}
	if a.svc != nil && a.svc.cache != nil {
		a.svc.cache.SetLimitMB([]int{0, 200, 500, 1000, 2000}[a.cacheLimit])
	}
}

// applyState 把主线 State 快照写入 app 状态字段（UI 兼容，读 a.current/a.playlist 等不变）。
func (a *app) applyState(st playback.State) {
	a.current = &track{bvid: st.BVID, title: st.Title, author: st.Artist, pic: st.Cover}
	a.playing = st.Playing
	a.playlist = toSearchItems(st.Queue)
	a.playIndex = st.QIdx
	a.loopMode = modeToInt(st.PlayMode)
	a.volume = float64(st.Volume) / 100.0
	if st.Duration > 0 {
		f := st.Position / st.Duration
		if f < 0 {
			f = 0
		}
		if f > 1 {
			f = 1
		}
		a.progress = f
	}
	a.posText = fmtDurMygo(st.Position)
	a.durText = fmtDurMygo(st.Duration)
	a.posSec = st.Position
	if st.Title != "" {
		a.lyricText = "♪ " + st.Title
	}
	a.win.Invalidate()
}

// modeToInt 主线 PlayMode → MyGo loopMode（0 列表/1 单曲/2 随机）。
func modeToInt(m string) int {
	switch m {
	case "loop":
		return 1
	case "single":
		return 2
	case "random":
		return 3
	default:
		return 0
	}
}

// intToMode MyGo loopMode → 主线 PlayMode（命令侧同步 SSOT）。
func intToMode(n int) string {
	switch n {
	case 1:
		return "loop"
	case 2:
		return "single"
	case 3:
		return "random"
	default:
		return "order"
	}
}

func toSearchItems(q []playback.Song) []bilibili.SearchItem {
	out := make([]bilibili.SearchItem, 0, len(q))
	for _, s := range q {
		out = append(out, bilibili.SearchItem{BVID: s.BVID, Title: s.Title, Author: s.Artist, Pic: s.Cover})
	}
	return out
}

// toSongList []bilibili.SearchItem → []playback.Song（SetQueue 命令用）。
func toSongList(list []bilibili.SearchItem) []playback.Song {
	out := make([]playback.Song, 0, len(list))
	for _, it := range list {
		out = append(out, toSong(it))
	}
	return out
}

// toSong bilibili.SearchItem → 主线 playback.Song（播放命令用）。
func toSong(it bilibili.SearchItem) playback.Song {
	return playback.Song{
		BVID:   it.BVID,
		Title:  bilibili.CleanHTML(it.Title),
		Artist: it.Author,
		Cover:  it.Pic,
	}
}
