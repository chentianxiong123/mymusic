//go:build native

// 歌词桥：复用主线 lyric 域包（网易云词源）——候选搜索/取行/偏移，SSOT 无副本。
package main

import (
	"log"
	"regexp"
	"strings"

	"homecast/service/lyric"
	"homecast/service/playlist"
	"homecast/service/widget"

	"mymusic-mygo/plugin"
)

// loadLyricAsync 后台加载歌词：走主线 LyricHandler 端点（/api/v1/music/lyric），
// 与主线 web 端 loadBackendLyric 完全同源——服务端 keywordsFor（Music.LyricKeywords
// 分P标题优先）→ LyricCache 透明缓存 → SelectedSID/GetLyricLines 自动最优。
// bvid 变化/重放时重新拉取；结果经 win.Update 回填渲染。
func (a *app) loadLyricAsync(bvid string) {
	if a.lyricLoading {
		return
	}
	a.lyricLoading = true
	defer func() { a.lyricLoading = false }()

	go func() {
		if a.current == nil {
			return
		}
		// 关键词对齐主线 lyric.js load()：CleanSearchKeyword(标题) + ' ' + 歌手
		// （服务端 bvid 非空会用分P标题覆盖，这边只是 fallback）
		kwRaw := lyric.CleanSearchKeyword(a.current.title) + " " + lyric.CleanSearchKeyword(a.current.author)
		kw := urlQE(kwRaw)
		// 歌词按源：歌来源有 JS lyric 插件（咪咕/QQ/酷我等）→ 用该源歌词；无 → 主线端点（网易云兜底）。
		if prov := plugin.Lyric(a.current.src); prov != nil {
			// sid 传歌 id 字符串（咪咕 contentId 18 位超 JS 安全整数，必须字符串保精度）。
			lines, lerr := prov.Fetch([]string{lyric.CleanSearchKeyword(a.current.title), lyric.CleanSearchKeyword(a.current.author)}, a.current.bvid)
			if lerr == nil && len(lines) > 0 {
				// source.LyricLine{Time,Text} → 主线 lyric.LyricLine [3]any{time,text,0}。
				srcLines := make([]lyric.LyricLine, len(lines))
				for i, ln := range lines {
					srcLines[i] = lyric.LyricLine{ln.Time, ln.Text, 0}
				}
				a.win.Update(func() {
					if a.current == nil || a.current.bvid != bvid {
						return
					}
					a.lyricLines = srcLines
					a.lyricSID = 0
					a.lyricOffset = 0
					log.Printf("[lyric] %s → 源歌词 %s (%d 行)", bvid, a.current.src, len(srcLines))
					a.win.Invalidate()
				})
				return
			}
			log.Printf("[lyric] %s 源歌词 %s 失败: %v", bvid, a.current.src, lerr)
		}
		// 1) 选定源行列表（同挂件/歌词页：服务端选源+透明缓存权威）
		res, err := a.lyricGetBVID(bvid, kw)
		if err != nil {
			log.Printf("[lyric] %s 取词失败: %v", bvid, err)
		}
		// 2) 候选栏元数据（交互数据；行显示权威走后端选定源）
		cands, cerr := a.lyricCandsViaHTTP(kw, bvid, 8)
		if cerr != nil {
			log.Printf("[lyric] %s 候选失败: %v", bvid, cerr)
		}
		a.win.Update(func() {
			if a.current == nil || a.current.bvid != bvid {
				return
			}
			if len(cands) > 0 {
				a.lyricCands = cands
			}
			if res != nil && len(res.Lines) > 0 {
				a.lyricLines = res.Lines
				a.lyricSID = res.ID
				a.lyricOffset = res.Offset
				log.Printf("[lyric] %s → sid=%d (%d 行)", bvid, res.ID, len(res.Lines))
			} else {
				a.lyricLines = nil
				a.lyricSID = 0
			}
			a.win.Invalidate()
		})
	}()
}

// linesAnyMygo []lyric.LyricLine → [][3]any（widget.PickLineIndex 入参类型桥；
// 类型不同（数组 vs 切片）但同构，仅转换无逻辑）。
func linesAnyMygo(lines []lyric.LyricLine) [][3]any {
	out := make([][3]any, len(lines))
	for i, ln := range lines {
		out[i] = ln
	}
	return out
}

// lyricAt 当前秒对应的歌词文本+索引（未匹配返回空串）。
// 定位复用主线 widget.PickLineIndex（sec<=t 最后一行，唯一实现住后端；主线改定位
// mygo 自动接上）。旧版自创循环（还带 0.3s 容差+int 截断）已删。
func (a *app) lyricAt(sec float64) (string, int) {
	idx, line := widget.PickLineIndex(linesAnyMygo(a.lyricLines), sec+a.lyricOffset)
	return line, idx
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
	if a.current == nil {
		return
	}
	// 选源上报主线端点（POST /lyric/select：SQLite 持久化 + 清该歌歌词缓存 + 广播），
	// 与主线 web 端 lyric.js selectSource 同源（选源变 → 后端重拉行列表）。
	var cand *lyric.LyricResult
	for i := range a.lyricCands {
		if a.lyricCands[i].ID == sid {
			cand = &a.lyricCands[i]
			break
		}
	}
	if err := a.lyricSelect(bvid, cand); err != nil {
		log.Printf("[lyric] select %d 失败: %v", sid, err)
	}
	kwRaw := lyric.CleanSearchKeyword(a.current.title) + " " + lyric.CleanSearchKeyword(a.current.author)
	res, err := a.lyricGetBVID(bvid, urlQE(kwRaw))
	if err != nil {
		log.Printf("[lyric] 选源后重拉失败: %v", err)
	}
	a.win.Update(func() {
		if a.current == nil || a.current.bvid != bvid {
			return
		}
		if res != nil && len(res.Lines) > 0 {
			a.lyricSID = res.ID
			a.lyricLines = res.Lines
			a.lyricOffset = res.Offset
		}
		a.win.Invalidate()
	})
}

// lyricTimeStr 秒 → m:ss（对齐主线歌词时间戳显示）。
// lyricTimeStr 秒 → m:ss。复用主线 playlist.FormatDuration（同公式，主线改自动接上）。
func lyricTimeStr(t float64) string {
	return playlist.FormatDuration(int(t))
}

// nudgeLyric 歌词对齐微调 ±0.5s（对齐主线 ly-nudge 逻辑；SSOT 持久化 offset）。
func (a *app) nudgeLyric(delta float64) {
	a.lyricOffset += delta
	a.setLyricOffset(a.lyricOffset)
}

// setLyricOffset 设置歌词对齐偏移（含归零）：POST /lyric/offset 主线端点持久化
// （SQLite + 清缓存 + 广播，与主线 web 端 ly-nudge 同源）。
func (a *app) setLyricOffset(v float64) {
	a.lyricOffset = v
	if a.current != nil {
		if err := a.lyricSetOffsetHTTP(a.current.bvid, v); err != nil {
			log.Printf("[lyric] offset %f 失败: %v", v, err)
		}
	}
	a.win.Invalidate()
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
	// 当前行定位复用主线 widget.PickLineIndex（唯一实现住后端）；进度计算是显示层
	// 逻辑（对齐主线 lyric.js sync：yrc 自带时长 l[2]，LRC 用下一行时间差估算），
	// 主线 Go 侧无此函数（前端 JS 算），mygo 保留公式但行定位不重复实现。
	_, idx := a.lyricAt(sec)
	if idx < 0 {
		return -1, 0
	}
	t := sec + a.lyricOffset
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
// 对齐主线 title.js hcTitle：提取《》（）【】「」[] 五种括号内容，多个用「 · 」拼接（前后端一致）。
func titleToSongMygo(t string) string {
	parts := []string{}
	re := regexp.MustCompile(`《([^》]+)》|（([^）]+)）|【([^】]+)】|「([^」]+)」|\[([^\]]+)\]`)
	for _, m := range re.FindAllStringSubmatch(t, -1) {
		for _, g := range m[1:] {
			if c := strings.TrimSpace(g); c != "" {
				parts = append(parts, c)
				break // 取第一个非空捕获组（五种括号之一）
			}
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, " · ")
	}
	return strings.TrimSpace(t)
}

// candDurText 候选时长 mm:ss（对齐主线 candDur；无则 --:--）。
func candDurText(dur int) string {
	if dur <= 0 {
		return "--:--"
	}
	return playlist.FormatDuration(dur)
}
