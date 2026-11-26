//go:build native

// plugin_total_test.go —— 插件 total 语义回归：有真 total 源读 API 字段，
// 无 total 源（audius/ccmixter）返回 0（未知），UI 降级为上一页/下一页。
package main

import (
	"os"
	"strings"
	"testing"
)

func TestPluginTotalSemantics(t *testing.T) {
	plugins := map[string]bool{
		"audius":   false, // API 无 total → 必须返回 0
		"ccmixter": false,
		"netease":  true, // 有真 total 字段
		"kugou":    true,
		"kuwo":     true,
		"migu":     true,
		"qqmusic":  true,
		"jamendo":  true,
		"archive":  true,
		"qianqian": true,
	}
	for name, hasRealTotal := range plugins {
		b, err := os.ReadFile("plugins/" + name + "/plugin.js")
		if err != nil {
			t.Fatalf("读插件 %s: %v", name, err)
		}
		s := string(b)
		if !hasRealTotal {
			// 无 total 源：search 返回值必须是 total: 0，不能是 songs.length
			if strings.Contains(s, "total: songs.length") {
				t.Errorf("%s 仍用 songs.length 当 total（假总数）", name)
			}
			if !strings.Contains(s, "return { total: 0, songs: songs };") {
				t.Errorf("%s 未返回 total: 0（未知总数）", name)
			}
		}
	}
}
