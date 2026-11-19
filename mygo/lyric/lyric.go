// Package lyric 歌词源插件：网易云歌词（复用主线 lyric 域，包装成插件）。
// 一切皆插件：歌词源也走插件注册表，与 B站音乐源并列。
package lyric

import (
	"strconv"

	"homecast/service/lyric"
	"mymusic-mygo/model"
)

// Provider 歌词源接口（插件化）。
// keywords=备选标题关键词（B站分P标题优先），sid>0=网易云精确歌单。
type Provider interface {
	Name() string // "netease"
	Fetch(keywords []string, sid string) ([]model.LyricLine, error)
}

// Netease 网易云歌词源（复用主线 lyric.GetLyricLines 纯函数，不重写词源）。
type Netease struct{}

// NewNetease 创建网易云歌词插件。
func NewNetease() *Netease { return &Netease{} }

func (p *Netease) Name() string { return "netease" }

// Fetch 包装主线 lyric.GetLyricLines：备选标题逐个搜 → 首个有词的候选。
func (p *Netease) Fetch(keywords []string, sid string) ([]model.LyricLine, error) {
	sidInt, _ := strconv.Atoi(sid)
	res := lyric.GetLyricLines(keywords, sidInt)
	if res == nil {
		return nil, nil
	}
	out := make([]model.LyricLine, len(res.Lines))
	for i, ln := range res.Lines {
		sec, _ := ln[0].(float64)
		text, _ := ln[1].(string)
		out[i] = model.LyricLine{Time: sec, Text: text}
	}
	return out, nil
}
