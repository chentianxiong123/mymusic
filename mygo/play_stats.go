//go:build native

// play_stats.go —— 播放统计（按歌/歌手聚合，本地 SQLite settings JSON）。
// 记录每次播放的开始次数与累计秒数；数据同时喂本地推荐（常听歌手/歌）。
// 存储：settings 键 mygo_playstats（JSON，结构 playStats）。
package main

import (
	"encoding/json"
	"sort"
	"time"

	"homecast/service/conv"
)

const playStatsKey = "mygo_playstats"

// playStat 一首歌的统计（跨源用 source+id 标识，不塞 BVID 撒谎）。
type playStat struct {
	Source   string  `json:"source"`
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Artist   string  `json:"artist"`
	Cover    string  `json:"cover"`
	Count    int    `json:"count"`
	LastAt   int64  `json:"lastAt"` // Unix 秒
}

type playStats struct {
	Songs []playStat `json:"songs"`
}

func (a *app) loadPlayStats() playStats {
	if a.db == nil {
		return playStats{}
	}
	v, ok := conv.LoadSetting(a.db, playStatsKey)
	if !ok || v == "" {
		return playStats{}
	}
	var s playStats
	if json.Unmarshal([]byte(v), &s) != nil {
		return playStats{}
	}
	return s
}

func (a *app) savePlayStats(s playStats) {
	if a.db == nil {
		return
	}
	b, err := json.Marshal(s)
	if err != nil {
		return
	}
	_ = conv.SaveSetting(a.db, playStatsKey, string(b))
}

// statsPlay 记录一次播放开始（计数 +1，歌/歌手信息刷新）。
func (a *app) statsPlay(source, id, title, artist, cover string) {
	s := a.loadPlayStats()
	found := false
	for i := range s.Songs {
		if s.Songs[i].Source == source && s.Songs[i].ID == id {
			s.Songs[i].Count++
			s.Songs[i].LastAt = time.Now().Unix()
			s.Songs[i].Title = title
			s.Songs[i].Artist = artist
			if cover != "" {
				s.Songs[i].Cover = cover
			}
			found = true
			break
		}
	}
	if !found {
		s.Songs = append(s.Songs, playStat{
			Source: source, ID: id, Title: title, Artist: artist,
			Cover: cover, Count: 1, LastAt: time.Now().Unix(),
		})
	}
	a.savePlayStats(s)
}

// topSongs 播放次数最高的 N 首（本地推荐用）。
func (a *app) topSongs(n int) []playStat {
	s := a.loadPlayStats()
	out := append([]playStat(nil), s.Songs...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].LastAt > out[j].LastAt
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

// topArtists 按歌手聚合播放次数（本地推荐用）。
type artistStat struct {
	Name  string
	Count int
}

func (a *app) topArtists(n int) []artistStat {
	s := a.loadPlayStats()
	m := map[string]int{}
	for _, song := range s.Songs {
		if song.Artist != "" {
			m[song.Artist] += song.Count
		}
	}
	out := make([]artistStat, 0, len(m))
	for k, v := range m {
		out = append(out, artistStat{k, v})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}
