// Package model 共享底层类型（音源/歌词/插件/UI 通用——谁都能用，非插件层）。
package model

// LyricLine 歌词行（音源无关；source/lyric/plugin/link 共用，抽这里避免 歌词域→音源域 越层）。
type LyricLine struct {
	Time float64 // 秒
	Text string
}
