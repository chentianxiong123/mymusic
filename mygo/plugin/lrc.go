package plugin

import (
	"regexp"
	"strconv"
	"strings"

	"mymusic-mygo/source"
)

// lrcRe LRC 行： [mm:ss.xx]text（宿主统一解析，别让每个 JS 插件重复实现）。
var lrcRe = regexp.MustCompile(`\[(\d+):(\d+(?:\.\d+)?)\](.*)`)

// parseLRC 解析 LRC 文本为 LyricLine（时间秒 + 文本）。
// 格式稳定，宿主统一处理；JS 插件只管返回 lrc 原文。
func parseLRC(text string) []source.LyricLine {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	var out []source.LyricLine
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		m := lrcRe.FindStringSubmatch(line)
		if len(m) != 4 {
			continue
		}
		mm, err1 := strconv.Atoi(m[1])
		ss, err2 := strconv.ParseFloat(m[2], 64)
		if err1 != nil || err2 != nil {
			continue
		}
		out = append(out, source.LyricLine{Time: float64(mm)*60 + ss, Text: strings.TrimSpace(m[3])})
	}
	return out
}
