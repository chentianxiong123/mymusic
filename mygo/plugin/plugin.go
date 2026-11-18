// Package plugin 插件系统：第三方按接口写插件，注册新音源/新目标。
//
// 插件约定（Go 惯用编译期注册，无 cgo 不依赖动态加载）：
//  1. 插件包实现 source.MusicSource 或 target.Target（或两者）
//  2. 插件包 init() 里调 RegisterSource/RegisterTarget 注册自己
//  3. 主程序 import _ "插件包路径" 即可启用（空导入触发 init）
//
// 未来支持动态加载（.so/WASM）可扩展 LoadFromDir 扫描插件目录，
// 但需保持无 cgo（WASM 优先）。
package plugin

import (
	"mymusic-mygo/lyric"
	"mymusic-mygo/source"
	"mymusic-mygo/target"
)

// 全局注册表（内置 + 第三方插件共享；启动时初始化）
var (
	sources = map[string]source.MusicSource{}
	targets = map[string]target.Target{}
	order   []string // 注册顺序（源展示顺序）
)

// RegisterSource 注册音源插件（init 里调）。同名覆盖（后注册胜）。
func RegisterSource(s source.MusicSource) {
	if _, ok := sources[s.Name()]; !ok {
		order = append(order, "source:"+s.Name())
	}
	sources[s.Name()] = s
}

// RegisterTarget 注册目标插件（init 里调）。同名覆盖。
func RegisterTarget(t target.Target) {
	if _, ok := targets[t.Name()+"/"+t.DeviceID()]; !ok {
		order = append(order, "target:"+t.Name())
	}
	targets[t.Name()+"/"+t.DeviceID()] = t
}

// Sources 已注册音源（启动后 UI 选源用）。
func Sources() map[string]source.MusicSource { return sources }

// Targets 已注册目标（投送目标列表）。
func Targets() map[string]target.Target { return targets }

// SourceNames 已注册音源名（按注册顺序）。
func SourceNames() []string {
	var out []string
	for _, k := range order {
		if len(k) > 7 && k[:7] == "source:" {
			out = append(out, k[7:])
		}
	}
	return out
}

// --- 歌词插件注册（一切皆插件：音源/歌词/目标三类并列） ---

// lyrics 歌词插件注册表（网易云/未来插件词源）。
var lyrics = map[string]lyric.Provider{}

// RegisterLyric 注册歌词插件（init 里调）。同名覆盖。
func RegisterLyric(l lyric.Provider) { lyrics[l.Name()] = l }

// Lyric 取指定歌词插件（网易云默认）。
func Lyric(name string) lyric.Provider {
	if name == "" {
		name = "netease" // 默认网易云
	}
	return lyrics[name]
}

// LyricNames 已注册歌词插件名。
func LyricNames() []string {
	var out []string
	for k := range lyrics {
		out = append(out, k)
	}
	return out
}
