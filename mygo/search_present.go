//go:build native

// search_present.go —— 搜索结果的「呈现层」处理：过滤 + 排序只作用于当前页，
// 不进数据接口（source.MusicSource.Search 返回 API 原样 songs + total）。
//
// 这样所有音源的数据契约统一：分页严格靠 API（total / size），
// B站特有的非歌过滤（cover/翻唱/教学/电台…）和 rank 排序只是 UI 单页的展示策略。
package main

import (
	"sort"

	"homecast/service/music"
	"mymusic-mygo/source"
)

// presentSongs 对本页结果做呈现处理：过滤非歌 + rank 排序（同分短歌优先）。
// 只在 UI 渲染前调用，不修改 grp.songs 本身（分页/总数仍按 API 原样）。
func presentSongs(songs []source.Song) []source.Song {
	if len(songs) == 0 {
		return songs
	}
	out := make([]source.Song, 0, len(songs))
	for _, s := range songs {
		if music.IsSong(s.Typename, s.Title, s.DurationSec) {
			out = append(out, s)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri := music.SongRank(out[i].Typename, out[i].Title, out[i].DurationSec)
		rj := music.SongRank(out[j].Typename, out[j].Title, out[j].DurationSec)
		if ri != rj {
			return ri < rj
		}
		return out[i].DurationSec < out[j].DurationSec
	})
	return out
}
