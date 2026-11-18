//go:build native

// 主线域包接入：收藏持久化 / 歌词选源 / 歌词缓存 / 投屏 / 音箱。
// 全部复用主线 service 包（SSOT），MyGo 只做 UI 渲染 + 命令调用。
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/lsongdev/miservice-go/miservice"
	"homecast/service/cache"
	"homecast/service/cast"
	"homecast/service/fav"
	"homecast/service/lyric"
	"homecast/service/lyriccache"
	"homecast/service/music"
	"homecast/service/sites"
	"homecast/service/songlist"
	"homecast/service/speaker"
	"homecast/service/token"
	"homecast/service/transcode"
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
	qrAuth     *xiaoai.SpeakerAuth       // 与 speaker 共用同一 auth（主线程组装，避免双实例）
	sites      *sites.SitesService       // 站点嗅探配置（SSOT：~/.config/homecast/sites.json）
	music      *music.MusicService       // 音乐域（B站搜索/取流/非歌过滤，供给端点用 GetAudioStream）
	xcoder     *transcode.Transcoder     // 音箱软转码 mp3 秒开（供给端点 /stream 用）
	songlist   *songlist.SonglistService // 歌单（用户自建、可命名、可多份；与 playlist=队列 区分）
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

// durationSec 时长文本 → 秒（收藏条目用；无则 0）。
// 复用主线 music.ParseDurationSec（m:ss / h:mm:ss 都支持，主线改解析 mygo 自动跟上）。
func durationSec(progress float32, durText string) int {
	return music.ParseDurationSec(durText)
}

// newServices 构造主线域包（对齐主线 server.go 的组装方式）。
func newServices(db *sql.DB, client *bilibili.Client, audioDir string) *services {
	tokens := token.NewTokenStore()
	spAuth := xiaoai.NewSpeakerAuth() // 单一 auth 实例：QR 登录后与 speaker 共享
	// maudio-mp3 转码缓存目录（音箱秒开 mp3；与音频缓存同根目录）
	mp3Dir := audioDir + "-mp3"
	if audioDir == "" {
		mp3Dir = ""
	}
	return &services{
		fav:        fav.NewFavService(db),
		lyricStore: lyric.NewLyricStore(db),
		lyricCache: lyriccache.NewLyricCache(db, -1), // -1=默认上限
		cast:       cast.NewCastService(client, tokens),
		speaker:    speaker.NewSpeakerService(spAuth, client, tokens),
		tokens:     tokens,
		cache:      cache.NewCacheService(db, audioDir, -1), // -1=默认上限
		qr:         xiaoai.NewQRLogin(),
		qrAuth:     spAuth,
		sites:      sites.NewSitesService(""), // 默认 ~/.config/homecast
		music:      &music.MusicService{Client: client},
		xcoder:     transcode.NewTranscoder(db, mp3Dir, -1), // -1=默认上限
		songlist:   songlist.NewSonglistService(db),
	}
}

// applyQRToken 扫码成功后把 QR token 灌进 speaker auth（对齐主线 hx/speaker.go applyQRToken）。
func (s *services) applyQRToken() {
	if s.qr == nil || s.qrAuth == nil {
		return
	}
	uid, pwd, ssec, stok, ok := s.qr.BuildTokens()
	if !ok {
		return
	}
	t := miservice.NewTokens()
	t.UserId = uid
	t.PassToken = pwd
	t.DeviceId = s.qr.DeviceID()
	t.Sids["micoapi"] = miservice.SidToken{Ssecurity: ssec, ServiceToken: stok}
	_ = s.qrAuth.LoginWithToken(t)
}

// speakerLoggedIn 音箱是否已登录（面板显示已登录布局用）。
func (s *services) speakerLoggedIn() bool {
	return s.qrAuth != nil && s.qrAuth.IsLoggedIn()
}

// speakerUserID 音箱登录用户 ID。
func (s *services) speakerUserID() string {
	if s.qrAuth == nil {
		return ""
	}
	return s.qrAuth.UserID()
}

// importSpeakerToken 粘贴 token 登录（对齐主线 /api/v1/speaker/token/import：
// 解析 miservice.Tokens → LoginWithToken → RefreshDevices）。多端免扫码复用。
func (s *services) importSpeakerToken(token string) error {
	if s.qrAuth == nil {
		return fmt.Errorf("音箱 auth 未就绪")
	}
	var t miservice.Tokens
	if err := json.Unmarshal([]byte(token), &t); err != nil {
		return fmt.Errorf("token 解析失败: %w", err)
	}
	if err := s.qrAuth.LoginWithToken(&t); err != nil {
		return fmt.Errorf("token 登录失败: %w", err)
	}
	_, _ = s.speaker.RefreshDevices()
	return nil
}

// exportSpeakerToken 导出 token JSON（多端复用，对齐主线 /api/v1/speaker/token/export）。
func (s *services) exportSpeakerToken() (string, error) {
	if s.qrAuth == nil {
		return "", fmt.Errorf("未登录，无 token 可导出")
	}
	t := s.qrAuth.Tokens()
	if t == nil {
		return "", fmt.Errorf("未登录，无 token 可导出")
	}
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
