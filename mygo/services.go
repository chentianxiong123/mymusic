//go:build native

// 主线域包接入：收藏持久化 / 歌词选源 / 歌词缓存 / 投屏 / 音箱。
// 全部复用主线 service 包（SSOT），MyGo 只做 UI 渲染 + 命令调用。
package main

import (
	"database/sql"
	"fmt"

	"homecast/service/cache"
	"homecast/service/cast"
	"homecast/service/fav"
	"homecast/service/lyric"
	"homecast/service/lyriccache"
	"homecast/service/speaker"
	"homecast/service/token"
	"homecast/xiaoai"

	"homecast/bilibili"
)

// services 主线域包句柄集合（MyGo 复用，不重造业务）。
type services struct {
	fav        *fav.FavService
	lyricStore *lyric.LyricStore
	lyricCache *lyriccache.LyricCache
	cast       *cast.CastService
	speaker    *speaker.SpeakerService
	tokens     *token.TokenStore
	cache      *cache.CacheService
	qr         *xiaoai.QRLogin
}

// favHas 收藏查询（薄封装，UI 侧只读）。
func (s *services) favHas(bvid string) bool {
	for _, f := range s.fav.List() {
		if f.BVID == bvid {
			return true
		}
	}
	return false
}

// favNewEntry 构造收藏条目（FavSong）。
func favNewEntry(bvid, title, artist, cover string, durSec int) fav.FavSong {
	return fav.FavSong{BVID: bvid, Title: title, Artist: artist, Cover: cover, DurationSec: durSec}
}

// durationSec 由进度+时长文本估算秒数（收藏条目用；无则 0）。
func durationSec(progress float32, durText string) int {
	// 简单解析 m:ss
	var m, s int
	if _, err := fmt.Sscanf(durText, "%d:%d", &m, &s); err == nil {
		return m*60 + s
	}
	return 0
}

// newServices 构造主线域包（对齐主线 server.go 的组装方式）。
func newServices(db *sql.DB, client *bilibili.Client, audioDir string) *services {
	tokens := token.NewTokenStore()
	return &services{
		fav:        fav.NewFavService(db),
		lyricStore: lyric.NewLyricStore(db),
		lyricCache: lyriccache.NewLyricCache(db, -1), // -1=默认上限
		cast:       cast.NewCastService(client, tokens),
		speaker:    speaker.NewSpeakerService(xiaoai.NewSpeakerAuth(), client, tokens),
		tokens:     tokens,
		cache:      cache.NewCacheService(db, audioDir, -1), // -1=默认上限
		qr:         xiaoai.NewQRLogin(),
	}
}
