package plugin

import (
	"fmt"

	"mymusic-mygo/lyric"
	"mymusic-mygo/source"
)

// jsSource JS 音源适配器：把 .js 的 search/getURL/lyric 包装成 source.MusicSource。
type jsSource struct{ jp *JSPlugin }

// 编译期契约：jsSource 必须实现 source.MusicSource。
var _ source.MusicSource = (*jsSource)(nil)

func (s *jsSource) Name() string { return s.jp.meta.Key }

// jsSong JS 侧歌曲字段（camelCase，对齐交接文档 §4）。
type jsSong struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	Cover       string `json:"cover"`
	Duration    string `json:"duration"`
	DurationSec int    `json:"durationSec"`
	Typename    string `json:"typename"`
	Album       string `json:"album"`
	VIP         bool   `json:"vip"`
}

// Search 调 JS search(ctx) → {total, songs:[{...}]}。
func (s *jsSource) Search(keyword string, page, size int) ([]source.Song, int, error) {
	ctx, err := s.jp.makeCtx(map[string]interface{}{
		"query": keyword, "page": page, "size": size,
	})
	if err != nil {
		return nil, 0, err
	}
	val, err := s.jp.call(s.jp.searchFn, ctx)
	if err != nil {
		return nil, 0, err
	}
	var out struct {
		Total int      `json:"total"`
		Songs []jsSong `json:"songs"`
		Error string   `json:"error"`
	}
	if err := s.jp.host.decode(val, &out); err != nil {
		return nil, 0, err
	}
	if out.Error != "" {
		return nil, 0, fmt.Errorf("%s", out.Error)
	}
	songs := make([]source.Song, 0, len(out.Songs))
	for _, js := range out.Songs {
		songs = append(songs, source.Song{
			ID: js.ID, Title: js.Title, Artist: js.Artist, Cover: js.Cover,
			Duration: js.Duration, DurationSec: js.DurationSec, Typename: js.Typename,
			Album: js.Album, VIP: js.VIP,
		})
	}
	return songs, out.Total, nil
}

// GetStream 调 JS getURL(ctx) → {url, quality, duration}。
func (s *jsSource) GetStream(id string, quality int) (*source.Stream, error) {
	ctx, err := s.jp.makeCtx(map[string]interface{}{
		"song":    map[string]interface{}{"id": id},
		"quality": quality,
	})
	if err != nil {
		return nil, err
	}
	val, err := s.jp.call(s.jp.getURLFn, ctx)
	if err != nil {
		return nil, err
	}
	var out struct {
		URL      string  `json:"url"`
		Quality  int     `json:"quality"`
		Duration float64 `json:"duration"`
		Error    string  `json:"error"`
	}
	if err := s.jp.host.decode(val, &out); err != nil {
		return nil, err
	}
	if out.Error != "" {
		return nil, fmt.Errorf("%s", out.Error)
	}
	if out.URL == "" {
		return nil, fmt.Errorf("getURL 返回空 url")
	}
	return &source.Stream{URL: out.URL, Quality: out.Quality, Duration: out.Duration}, nil
}

// GetLyric 调 JS lyric(ctx) → {lrc} → 宿主 parseLRC。
func (s *jsSource) GetLyric(id string, keywords []string, sid int) ([]source.LyricLine, error) {
	if s.jp.lyricFn == nil {
		return nil, nil
	}
	ctx, err := s.jp.makeCtx(map[string]interface{}{
		"song":     map[string]interface{}{"id": id},
		"keywords": keywords,
	})
	if err != nil {
		return nil, err
	}
	val, err := s.jp.call(s.jp.lyricFn, ctx)
	if err != nil {
		return nil, err
	}
	var out struct {
		LRC   string `json:"lrc"`
		Error string `json:"error"`
	}
	if err := s.jp.host.decode(val, &out); err != nil {
		return nil, err
	}
	if out.Error != "" {
		return nil, fmt.Errorf("%s", out.Error)
	}
	return parseLRC(out.LRC), nil
}

// jsLyric JS 歌词适配器（capabilities 含 lyric）：实现 lyric.Provider。
type jsLyric struct{ jp *JSPlugin }

// 编译期契约：jsLyric 必须实现 lyric.Provider。
var _ lyric.Provider = (*jsLyric)(nil)

func (l *jsLyric) Name() string { return l.jp.meta.Key }

// Fetch 调 JS lyric(ctx) → {lrc} → 宿主 parseLRC。
func (l *jsLyric) Fetch(keywords []string, sid string) ([]source.LyricLine, error) {
	if l.jp.lyricFn == nil {
		return nil, nil
	}
	ctx, err := l.jp.makeCtx(map[string]interface{}{
		"keywords": keywords, "sid": sid,
	})
	if err != nil {
		return nil, err
	}
	val, err := l.jp.call(l.jp.lyricFn, ctx)
	if err != nil {
		return nil, err
	}
	var out struct {
		LRC   string `json:"lrc"`
		Error string `json:"error"`
	}
	if err := l.jp.host.decode(val, &out); err != nil {
		return nil, err
	}
	if out.Error != "" {
		return nil, fmt.Errorf("%s", out.Error)
	}
	return parseLRC(out.LRC), nil
}
