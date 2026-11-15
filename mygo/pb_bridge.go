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
	"log"
	"os"
	"path/filepath"
	"time"

	"homecast/service/playback"
	"homecast/store"

	"mymusic-mygo/bilibili"
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
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS settings(key TEXT PRIMARY KEY, value TEXT)`); err != nil {
		log.Printf("[mygo] ensure settings table: %v", err)
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
	case "single":
		return 1
	case "random":
		return 2
	default:
		return 0
	}
}

// intToMode MyGo loopMode → 主线 PlayMode（命令侧同步 SSOT）。
func intToMode(n int) string {
	switch n {
	case 1:
		return "single"
	case 2:
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
