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

// loadLyricAsync 后台加载歌词：按「歌词插件页的配置」顺序+启用状态挨个试，
// 第一个出词的插件就用——所有歌词源完全平等（网易云无特权，只是默认排第一）。
// 没有「音源绑定」「兜底」概念；全关/全失败就没词。
// 网易云走 HTTP 端点（拿完整 Words/Offset/候选），JS 词源插件走 Fetch。
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
		kwRaw := lyric.CleanSearchKeyword(a.current.title) + " " + lyric.CleanSearchKeyword(a.current.author)
		kw := urlQE(kwRaw)
		keywords := []string{lyric.CleanSearchKeyword(a.current.title), lyric.CleanSearchKeyword(a.current.author)}

		// 歌词绑定：歌级 > 音源渠道级（音源插件详情页/歌曲右键可配置）。
		bound := a.lyricBoundForSong(a.current.src, a.current.bvid)
		infos := plugin.InfoNames("lyric")
		var chosen *plugin.Info
		for i := range infos {
			if infos[i].Name == bound && plugin.IsEnabled("lyric", bound) {
				chosen = &infos[i]
				break
			}
		}
		if chosen == nil {
			// 绑定缺失/被关 → 没词（不跨源兜底，配置归用户）
			a.win.Update(func() {
				if a.current == nil || a.current.bvid != bvid {
					return
				}
				a.lyricLines = nil
				a.lyricWords = nil
				a.lyricSID = 0
				a.lyricSrcName = ""
				a.win.Invalidate()
			})
			return
		}
		for _, in := range []plugin.Info{*chosen} {
			prov := plugin.Lyric(in.Name)
			if prov == nil {
				continue
			}

			var lines []lyric.LyricLine
			var words [][]lyric.WordSpan
			var offset float64
			var sid int
			var cands []lyric.LyricResult

			if in.Name == "netease" {
				// 网易云：走 HTTP 端点拿完整数据（selected lines + words + offset + 候选）
				res, err := a.lyricGetBVID(bvid, kw)
				if err != nil || res == nil || len(res.Lines) == 0 {
					if err != nil {
						log.Printf("[lyric] %s netease 取词失败: %v", bvid, err)
					}
					continue
				}
				lines, words, offset, sid = res.Lines, res.Words, res.Offset, res.ID
				if cr, cerr := a.lyricCandsViaHTTP(kw, bvid, 8); cerr == nil {
					cands = cr
				}
			} else {
				// 其它歌词插件：Fetch 返回 model.LyricLine{Time,Text}
				src, ferr := prov.Fetch(keywords, a.current.bvid)
				if ferr != nil || len(src) == 0 {
					if ferr != nil {
						log.Printf("[lyric] %s %s 取词失败: %v", bvid, in.Name, ferr)
					}
					continue
				}
				lines = make([]lyric.LyricLine, len(src))
				for i, ln := range src {
					lines[i] = lyric.LyricLine{ln.Time, ln.Text, 0.0}
				}
			}

			// 命中：回填 UI
			name := in.Name
			a.win.Update(func() {
				if a.current == nil || a.current.bvid != bvid {
					return
				}
				a.lyricLines = lines
				a.lyricWords = words
				a.lyricSID = sid
				a.lyricSrcName = name
				a.lyricOffset = offset
				if len(cands) > 0 {
					a.lyricCands = cands
				}
				log.Printf("[lyric] %s → %s (%d 行)", bvid, name, len(lines))
				a.win.Invalidate()
			})
			return
		}

		// 全部插件没出词 → 清空
		a.win.Update(func() {
			if a.current == nil || a.current.bvid != bvid {
				return
			}
			a.lyricLines = nil
			a.lyricWords = nil
			a.lyricSID = 0
			a.lyricSrcName = ""
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

// lyricSourceText 中栏来源行：显示实际出词的歌词插件名（插件页可配置顺序/开关）。
func (a *app) lyricSourceText() string {
	if len(a.lyricLines) == 0 {
		return "未找到歌词 — 去「插件 → 歌词」启用更多歌词源"
	}
	name := a.lyricSrcName
	if name == "" {
		name = "自动匹配"
	}
	if a.lyricSID > 0 {
		for _, c := range a.lyricCands {
			if c.ID == a.lyricSID {
				s := "来源：" + name + " · " + c.Artist
				if c.Artist != "" && c.Name != "" {
					s += "《" + c.Name + "》"
				} else if c.Name != "" {
					s += " " + c.Name
				}
				return s
			}
		}
	}
	return "来源：" + name + " · 自动匹配"
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
