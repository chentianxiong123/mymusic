//go:build native

// 歌词桥：复用主线 lyric 域包（网易云词源）——候选搜索/取行/偏移，SSOT 无副本。
package main

import (
	"fmt"
	"log"

	"homecast/service/lyric"

	"mymusic-mygo/bilibili"
)

// loadLyricAsync 后台加载歌词：搜候选 → 自动取最优（对齐主线 lyric.js 自动播放最优）。
// bvid 变化/重放时重新拉取；结果经 RunOnMain 回填渲染。
func (a *app) loadLyricAsync(bvid string) {
	if a.lyricLoading {
		return
	}
	a.lyricLoading = true
	defer func() { a.lyricLoading = false }()

	// 未播放中止（window 关闭后还在跑的 goroutine 不再回填）
	go func() {
		kw := bilibili.CleanHTML(a.current.title)
		if kw == "" {
			return
		}
		cands := lyric.SearchLyricCandidates(lyric.CleanSearchKeyword(kw), 6)
		if len(cands) == 0 {
			return
		}
		best := lyric.GetLyricLines([]string{kw}, cands[0].ID)
		if best == nil || len(best.Lines) == 0 {
			return
		}
		// 候选列表回填（别名/时长展示用）
		sel := cands[0]
		if sel.Name != "" && sel.Artist != "" {
			sel.Lines = best.Lines
			sel.Offset = best.Offset
		}
		a.win.Update(func() {
			if a.current == nil || a.current.bvid != bvid {
				return
			}
			a.lyricCands = cands
			a.lyricSID = best.ID
			a.lyricLines = best.Lines
			a.lyricOffset = best.Offset
			a.win.Invalidate()
			log.Printf("[lyric] %s → %s - %s (%d 行)", bvid, best.Name, best.Artist, len(best.Lines))
		})
	}()
}

// lyricAt 当前秒对应的歌词文本（未匹配返回空串；对齐主线 ktv 当前行逻辑）。
func (a *app) lyricAt(sec float64) (string, int) {
	if len(a.lyricLines) == 0 {
		return "", -1
	}
	t := sec + a.lyricOffset
	idx, cur := -1, ""
	for i, ln := range a.lyricLines {
		ls, ok := ln[0].(float64)
		if !ok {
			continue
		}
		if float64(int(t)) >= ls-0.3 {
			idx = i
			if s, ok := ln[1].(string); ok {
				cur = s
			}
		}
	}
	return cur, idx
}

// lyricLineText 第 i 行歌词文本。
func lyricLineText(ln lyric.LyricLine) string {
	if len(ln) > 1 {
		if s, ok := ln[1].(string); ok {
			return s
		}
	}
	return ""
}

// lyricLineTime 第 i 行开始秒。
func lyricLineTime(ln lyric.LyricLine) float64 {
	if len(ln) > 0 {
		if f, ok := ln[0].(float64); ok {
			return f
		}
	}
	return 0
}

// pickLyricSID 选择候选 sid（对齐主线 ly-cand-list 点选换源）。
func (a *app) pickLyricSID(bvid string, sid int) {
	kw := bilibili.CleanHTML(a.current.title)
	best := lyric.GetLyricLines([]string{kw}, sid)
	a.win.Update(func() {
		if a.current == nil || a.current.bvid != bvid {
			return
		}
		if best != nil {
			a.lyricSID = sid
			a.lyricLines = best.Lines
			a.lyricOffset = best.Offset
		}
		a.win.Invalidate()
	})
}

// lyricTimeStr 秒 → m:ss（对齐主线歌词时间戳显示）。
func lyricTimeStr(t float64) string {
	m := int(t) / 60
	sec := int(t) % 60
	return fmt.Sprintf("%d:%02d", m, sec)
}

// nudgeLyric 歌词对齐微调 ±0.5s（对齐主线 ly-nudge 逻辑）。
func (a *app) nudgeLyric(delta float64) {
	a.lyricOffset += delta
	a.win.Invalidate()
}
