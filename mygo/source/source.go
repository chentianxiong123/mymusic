// Package source 音源抽象：B站/网易云/本地/插件都实现 MusicSource。
// 核心原则：复用主线 music 域（MusicService 已封装 B站搜索/取流/缓存），
// 这里只做接口抽象，不重写业务逻辑。
package source

import (
	"homecast/bilibili"
	"homecast/service/lyric"
	"homecast/service/music"
	"mymusic-mygo/model"
)

// Song 统一歌曲条目（音源无关的规范结构）。
type Song struct {
	ID          string // 音源内唯一 id（B站=bvid，网易云=id，本地=路径）
	Title       string
	Artist      string
	Cover       string
	Duration    string
	DurationSec int
	Typename    string
	Album       string // 专辑名（网易云=album.name；B站无=空）
	VIP         bool   // 收费/会员标志（网易云 fee>0；QQ pay==1；B站无）
}

// Stream 音频流（音源无关；B站 DASH，本地=文件）。
type Stream struct {
	URL      string // 播放地址（B站 DASH baseUrl / 本地 file://）
	Quality  int    // 音质档 0/1/2
	Duration float64
	// 扩展字段（未来本地源用）
}

// LyricLine 歌词行（共享底层——model 包；避免 歌词域→音源域 越层）。
// type LyricLine = model.LyricLine

// MusicSource 音源接口：搜索 + 取流 + 歌词。
// UI/播放核心只认这个接口，不认具体源（B站/网易云/本地）。
type MusicSource interface {
	Name() string                                               // "bilibili" / "netease" / "local"
	Search(keyword string, page, size int) ([]Song, int, error) // 返回 (songs, total, err)
	GetStream(id string, quality int) (*Stream, error)
	GetLyric(id string, keywords []string, sid int) ([]model.LyricLine, error)
}

// --- B站实现：包装主线 music.MusicService（不重写 B站逻辑） ---

// BilibiliSource B站音源（包装主线 music 域）。
type BilibiliSource struct {
	svc *music.MusicService
}

// NewBilibili 创建 B站音源。
func NewBilibili(svc *music.MusicService) *BilibiliSource {
	return &BilibiliSource{svc: svc}
}

func (s *BilibiliSource) Name() string { return "bilibili" }

// Search 复用主线 music.Search（含 FilterSongs 过滤 + rank 排序 + 缓存）。
func (s *BilibiliSource) Search(keyword string, page, size int) ([]Song, int, error) {
	res, err := s.svc.Search(keyword, page, size)
	if err != nil {
		return nil, 0, err
	}
	songs := make([]Song, 0, len(res.List))
	for _, it := range res.List {
		songs = append(songs, Song{
			ID: it.BVID, Title: it.Title, Artist: it.Artist, Cover: it.Cover,
			Duration: it.Duration, DurationSec: it.DurationSec, Typename: it.Typename,
		})
	}
	return songs, res.Total, nil
}

// GetStream 复用主线 music.GetAudioStream（VideoInfo 缓存 + GetBestAudioURL）。
// quality = 音质档（0=64k 1=128k 2=192k，UI 层语义）；内部映射到 B站 qn 常量。
func (s *BilibiliSource) GetStream(id string, quality int) (*Stream, error) {
	qn := bilibili.PreferredQuality
	switch quality {
	case 0:
		qn = bilibili.Audio64K
	case 1:
		qn = bilibili.Audio128K
	}
	r, err := s.svc.GetAudioStream(id, qn)
	if err != nil {
		return nil, err
	}
	return &Stream{URL: r.URL, Quality: quality}, nil
}

// GetLyric B站歌词：主线 lyric 域（网易云词源）GetLyricLines 纯函数取词。
// keywords = 标题关键词 + 歌手；sid>0 精确取。返回歌词行（无词返回 nil）。
func (s *BilibiliSource) GetLyric(id string, keywords []string, sid int) ([]model.LyricLine, error) {
	res := lyric.GetLyricLines(keywords, sid, s.svc.LyricDuration(id))
	if res == nil {
		return nil, nil
	}
	out := make([]model.LyricLine, len(res.Lines))
	for i, ln := range res.Lines {
		// 主线 LyricLine = [3]any（[0]=秒, [1]=行文本, [2]=行时长）
		sec, _ := ln[0].(float64)
		text, _ := ln[1].(string)
		out[i] = model.LyricLine{Time: sec, Text: text}
	}
	return out, nil
}
