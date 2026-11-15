//go:build native

// 歌词桥：复用主线 lyric 域包（网易云词源）——候选搜索/取行/偏移，SSOT 无副本。
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"

	"homecast/service/lyric"

	"homecast/bilibili"
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
		// 上次选源优先（SSOT：lyric_sels 表，对齐主线 lyricStore）
		if a.svc != nil {
			if sid := a.svc.lyricStore.SelectedSID(bvid); sid > 0 {
				if sel, ok := a.svc.lyricStore.Get(bvid); ok && sel.BVID == bvid {
					if r := lyric.GetLyricLines([]string{kw}, sid); r != nil && len(r.Lines) > 0 {
						a.win.Update(func() {
							if a.current == nil || a.current.bvid != bvid {
								return
							}
							a.lyricSID = sid
							a.lyricLines = r.Lines
							a.lyricOffset = sel.Offset
							a.win.Invalidate()
						})
						a.cacheLyric(bvid, r.Lines)
						return
					}
				}
			}
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
		// SSOT：记忆选源（lyric_sels）+ 歌词行缓存（lyriccache）
		if a.svc != nil {
			a.svc.lyricStore.SetSelection(bvid, lyric.LyricSel{
				BVID: bvid, SID: best.ID, Name: best.Name, Artist: best.Artist,
				Cover: a.current.pic, Duration: best.Duration, Offset: best.Offset,
			})
			a.cacheLyric(bvid, best.Lines)
		}
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
			if a.svc != nil {
				a.svc.lyricStore.SetSelection(bvid, lyric.LyricSel{
					BVID: bvid, SID: sid, Name: best.Name, Artist: best.Artist,
					Cover: a.current.pic, Duration: best.Duration, Offset: best.Offset,
				})
			}
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

// nudgeLyric 歌词对齐微调 ±0.5s（对齐主线 ly-nudge 逻辑；SSOT 持久化 offset）。
func (a *app) nudgeLyric(delta float64) {
	a.lyricOffset += delta
	a.setLyricOffset(a.lyricOffset)
}

// setLyricOffset 设置歌词对齐偏移（含归零；SSOT：lyric_sels 表持久化）。
func (a *app) setLyricOffset(v float64) {
	a.lyricOffset = v
	if a.svc != nil && a.current != nil {
		a.svc.lyricStore.SetOffset(a.current.bvid, v)
	}
	a.win.Invalidate()
}

// cacheLyric 歌词行写入 lyriccache（SQLite，LRU 上限），对齐主线 lyricHandler。
func (a *app) cacheLyric(bvid string, lines []lyric.LyricLine) {
	if a.svc == nil || len(lines) == 0 {
		return
	}
	// 序列化行（[秒,文本,时长]）为 JSON 存 lyriccache
	b, err := json.Marshal(lines)
	if err != nil {
		return
	}
	_ = a.svc.lyricCache.Put(bvid, string(b))
}

// lyricSourceText 中栏来源行（对齐主线 lyric.js setSource：
// 「来源：网易云 · 歌手《歌名》」，无歌词给引导文案）。
func (a *app) lyricSourceText() string {
	if len(a.lyricLines) == 0 {
		return "未找到歌词 — 点候选栏换源"
	}
	if a.lyricSID > 0 {
		for _, c := range a.lyricCands {
			if c.ID == a.lyricSID {
				s := "来源：网易云 · " + c.Artist
				if c.Artist != "" && c.Name != "" {
					s += "《" + c.Name + "》"
				} else if c.Name != "" {
					s += " " + c.Name
				}
				return s
			}
		}
	}
	return "来源：网易云 · 自动匹配"
}

// lyricProg 当前行卡拉OK进度（对齐主线 lyric.js sync：当前行唱到百分之几，
// yrc 用自带时长 l[2]，LRC 用下一行时间差估算）。
// 返回当前行下标 + 0~1 进度；无歌词行时 -1,0。
func (a *app) lyricProg(sec float64) (int, float64) {
	if len(a.lyricLines) == 0 {
		return -1, 0
	}
	t := sec + a.lyricOffset
	idx := -1
	for i, ln := range a.lyricLines {
		ls, ok := ln[0].(float64)
		if !ok {
			continue
		}
		if t >= ls-0.3 {
			idx = i
		}
	}
	if idx < 0 {
		return -1, 0
	}
	start, _ := a.lyricLines[idx][0].(float64)
	dur := 0.0
	if d, ok := a.lyricLines[idx][2].(float64); ok && d > 0 {
		dur = d // yrc 逐字行自带时长
	} else if idx+1 < len(a.lyricLines) {
		if nxt, ok := a.lyricLines[idx+1][0].(float64); ok {
			dur = nxt - start
		}
	}
	if dur <= 0 {
		return idx, 1
	}
	p := (t - start) / dur
	if p < 0 {
		p = 0
	}
	if p > 1 {
		p = 1
	}
	return idx, p
}

// seekToLine 点击歌词行 → 跳到该句时间（对齐主线 renderLines 点击：
// currentTime = lines[i][0] - offset + 0.1，所见即所听）。
func (a *app) seekToLine(idx int) {
	if idx < 0 || idx >= len(a.lyricLines) {
		return
	}
	ls, ok := a.lyricLines[idx][0].(float64)
	if !ok {
		return
	}
	a.pb.Seek(ls - a.lyricOffset + 0.1)
}

// titleToSongMygo 歌名解析（对齐主线 lyric.js titleToSong：
// 剥 []【】取《》; 取不到用全标题）。
func titleToSongMygo(t string) string {
	clean := regexp.MustCompile(`[\[【][^\]】]*[\]】]`).ReplaceAllString(t, "")
	m := regexp.MustCompile(`《([^》]+)》`).FindStringSubmatch(clean)
	if m != nil {
		return strings.TrimSpace(m[1])
	}
	out := strings.TrimSpace(clean)
	if out == "" {
		return t
	}
	return out
}

// candDurText 候选时长 mm:ss（对齐主线 candDur；无则 --:--）。
func candDurText(dur int) string {
	if dur <= 0 {
		return "--:--"
	}
	return fmt.Sprintf("%d:%02d", dur/60, dur%60)
}
