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
	"homecast/service/songlist"
	"homecast/service/transcode"
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
		transcode.Schema, // 音箱 mp3 转码缓存表（供给端点 Xcoder 用）
		cache.SettingsSchema,
		lyriccache.CacheSchema,
		songlist.SonglistSchema,     // 歌单（用户自建、可命名、可多份）
		songlist.SonglistItemSchema, // 歌单条目
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
						if !a.scrubbing {
							a.progress = f // 拖动中不覆盖（避免 thumb 跳回）
						}
					}
					a.posText = fmtDurMygo(pr.Position)
					a.durText = fmtDurMygo(pr.Duration)
					a.posSec = pr.Position // 歌词/桌面挂件随进度走（音箱模式进度只经核心广播来）
					a.playing = pr.Playing
					// 只在播放中或播放态刚变化时重绘：暂停静止时 progress 帧仍在 250ms
					// 到达（progressReporter 上报），但内容没动 → 不 Invalidate，mygo 保持 0 帧。
					// 暂停期间的用户交互各自会 Invalidate，不依赖此循环。
					if pr.Playing || pr.Playing != a.lastPBPlaying {
						a.win.Invalidate()
					}
					a.lastPBPlaying = pr.Playing
				})
			}
		}
	}
}

// progressReporter 本地播放进度：直接读播放端 audio.Player，不再写入 SSOT。
//
// 进度是播放端的产物（Position() 从音频输出时钟插值算出），不是业务状态。
// 本地播放时 UI/歌词/挂件直接消费它，不必绕 SSOT 一圈（写 PB → 广播 → 回读）。
// 音箱模式（source=speaker）进度来自音箱设备，由核心 progressLoop 投影并广播，
// 本循环跳过——那才是该由后端管进度的场景。
func (a *app) progressReporter() {
	// 播放时长统计：按真实进度差累计（暂停不加、seek 大跳不计），10s 节流落库。
	// 之前固定每 10s 加 10 秒，暂停/切歌/seek 都会多算。

	// 250ms 拉播放端进度（audio.Player.Position 从输出时钟插值算，是播放端产物）。
	// 本循环只更新本地 UI 字段 + 推挂件 SSE，不再写 SSOT：进度不进业务状态源。
	// 音箱模式（source=speaker）进度来自设备，由核心 progressLoop 投影广播，这里跳过。
	t := time.NewTicker(250 * time.Millisecond)
	defer t.Stop()
	lastPlaying := false // 上次播放状态：暂停瞬间仍要重绘一次让 EQ 归位
	for range t.C {
		if a.spkMode {
			lastPlaying = false
			continue
		}
		info := a.mygoOut.Position()
		if info.Dur > 0 {
			a.win.Update(func() {
				if !a.scrubbing {
					f := info.Pos / info.Dur
					if f < 0 {
						f = 0
					}
					if f > 1 {
						f = 1
					}
					a.progress = f
				}
				a.posText = fmtDurMygo(info.Pos)
				a.durText = fmtDurMygo(info.Dur)
				a.posSec = info.Pos
				a.playing = info.Playing
			})
			a.pushWidgetState() // 挂件 SSE 歌词行随进度更新（去重，跨进程）
		}
		// dock 封面 EQ 频谱相位（播放中跳动；对齐主线 d-eq CSS 动画）
		if info.Playing {
			a.eqPhase++
		} else {
			a.eqPhase = -1 // -1 = 不显示
		}
		// 播放中持续重绘（进度条/歌词走 250ms）；暂停静止不重绘 → 0 帧空闲。
		if info.Playing || info.Playing != lastPlaying {
			a.win.Invalidate()
			if a.lyricWin != nil {
				a.lyricWin.Invalidate()
			}
		}
		lastPlaying = info.Playing
	}
}

// savePref 设置键值持久化（settings 表；对齐主线 localStorage hc:* 语义）。
// savePref 写设置整数（settings 唯一写入入口 conv.SaveSetting：主线改表自动跟上）。
func (a *app) savePref(key string, v int) {
	if a.db == nil {
		return
	}
	_ = conv.SaveSetting(a.db, key, fmt.Sprintf("%d", v))
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
	_ = conv.SaveSetting(a.db, "mygo_recent", string(data))
}

// restoreRecent 启动恢复最近播放。
func (a *app) restoreRecent() {
	if a.db == nil {
		return
	}
	v, ok := conv.LoadSetting(a.db, "mygo_recent")
	if !ok {
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
	_ = conv.SaveSetting(a.db, "mygo_history", string(data))
}

// restoreHistory 启动恢复搜索历史。
func (a *app) restoreHistory() {
	if a.db == nil {
		return
	}
	v, ok := conv.LoadSetting(a.db, "mygo_history")
	if !ok {
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
	a.cacheLimit = a.loadPref("mygo_cachelimit", 2)
	a.cacheCustom = a.loadPref("mygo_cachelimit_custom", 1500)
	a.lyricLimit = a.loadPref("mygo_lyriclimit", 2)
	a.lyricCustom = a.loadPref("mygo_lyriclimit_custom", 800)
	a.loopMode = a.loadPref("mygo_loopmode", 0)
	a.pointPlayMode = a.loadPref("mygo_pointplay", 0) // 默认替换列表（对齐网易云）
	a.lyricSize = a.loadPref("mygo_lyricsize", 2)
	a.lyricCtrlBottom = a.loadPref("mygo_lyricctrlpos", 1) == 1 // 默认下方
	a.androidLyricOn = a.loadPref("mygo_androlyric", 0) == 1    // 安卓悬浮歌词持久化
	if a.restoreCfg.Volume {
		a.volume = a.loadPrefFloat("mygo_volume", 0.8)
	}
	if a.restoreCfg.Muted {
		a.muted = a.loadPref("mygo_muted", 0) == 1
	}
	if a.svc != nil && a.svc.speaker != nil && a.loopMode != 0 {
		a.pb.SetMode(intToMode(a.loopMode))
	}
	if a.svc != nil && a.svc.cache != nil {
		a.svc.cache.SetLimitMB(a.cacheMB())
		a.svc.lyricCache.SetLimit(a.lyricCap())
	}
}

// applyState 把主线 State 快照写入 app 状态字段（UI 兼容，读 a.current/a.playlist 等不变）。
func (a *app) applyState(st playback.State) {
	// 切歌检测（BVID 变化）→ 自动加载歌词（对齐主线 lyric.js 切歌自动搜最优；
	// 之前只在歌词页打开时 loadLyricAsync → 首次进入系统歌词不自动刷新，进歌词页才刷）
	if a.current == nil || a.current.bvid != st.BVID {
		if st.BVID != "" && !a.lyricLoading {
			go a.loadLyricAsync(st.BVID)
		}
	}
	// 1.2.5 跨源引用：来源取歌曲自身（st.Source），不是输出端全局当前源。
	a.current = &track{bvid: st.BVID, title: st.Title, author: st.Artist, pic: st.Cover, src: st.Source}
	a.currentRef = st.Ref()
	// 同步「当前该播的 bvid」给输出端：启动恢复（pb restore）只写 p.cur 不调 Play，
	// 输出端 o.p==nil。不同步则首次点播放 Resume 无音源可恢复 → 没声音
	// （必须去队列/搜索重新点歌才放）。
	if a.mygoOut != nil {
		a.mygoOut.SetPending(st.BVID, st.Title)
	}
	a.playing = st.Playing
	a.spkMode = st.Source == "speaker" // 音箱模式：控制面复用主线（out 自动分派音箱）
	a.spkDid = st.SpkDID
	a.playlist = toSearchItems(st.Queue)
	a.playIndex = st.QIdx
	a.loopMode = modeToInt(st.PlayMode)
	// 音量：web 模式不被广播覆盖（st.Volume 主线 web 不管，覆盖会音量条归 0）；
	// 音箱模式（source=speaker）st.Volume 是音箱真实音量 → 回填音量条（对齐主线 state.js）。
	if a.spkMode && st.Volume >= 0 {
		a.volume = float64(st.Volume) / 100
	}
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
// 1.2.5：Src 决定跨源引用；空=bilibili（兼容老数据）。
func toSong(it bilibili.SearchItem) playback.Song {
	src := it.Src
	if src == "" {
		src = "bilibili"
	}
	return playback.Song{
		BVID:   it.BVID,
		Title:  bilibili.CleanHTML(it.Title),
		Artist: it.Author,
		Cover:  it.Pic,
		Source: src,
		ID:     it.BVID,
	}
}

// cacheMB 当前歌曲缓存档位对应 MB（4=自定义）。
func (a *app) cacheMB() int {
	if a.cacheLimit == 4 {
		return a.cacheCustom
	}
	return []int{0, 200, 500, 1000}[clamp4(a.cacheLimit)]
}

// lyricCap 当前歌词缓存档位对应首数（4=自定义）。
func (a *app) lyricCap() int {
	if a.lyricLimit == 4 {
		return a.lyricCustom
	}
	return []int{0, 100, 300, 500}[clamp4(a.lyricLimit)]
}

func clamp4(v int) int {
	if v < 0 || v > 3 {
		return 2
	}
	return v
}
