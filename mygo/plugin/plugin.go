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
	"log"

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
// 系统级（Go 内置）音源的 key 受保护：命中则拒绝覆盖并记日志（防御层；
// 主报错路径是 JS 插件加载期 CheckPluginKey）。
func RegisterSource(s source.MusicSource) {
	if IsBuiltinKey("source", s.Name()) {
		log.Printf("[plugin] 拒绝覆盖系统级音源 %q", s.Name())
		return
	}
	if _, ok := sources[s.Name()]; !ok {
		order = append(order, "source:"+s.Name())
	}
	sources[s.Name()] = s
}

// RegisterSystemSource 注册系统级（Go 内置）音源：key 必须在 systemCapabilities 已声明。
func RegisterSystemSource(s source.MusicSource) {
	ensureSystemKey("source", s.Name())
	if _, ok := sources[s.Name()]; !ok {
		order = append(order, "source:"+s.Name())
	}
	sources[s.Name()] = s
}

// RegisterTarget 注册目标插件（init 里调）。同名覆盖。
func RegisterTarget(t target.Target) {
	k := t.Name() + "/" + t.DeviceID()
	if IsBuiltinKey("target", t.Name()) {
		log.Printf("[plugin] 拒绝覆盖系统级目标 %q", t.Name())
		return
	}
	if _, ok := targets[k]; !ok {
		order = append(order, "target:"+t.Name())
	}
	targets[k] = t
}

// RegisterSystemTarget 注册系统级（Go 内置）播放目标：key 必须在 systemCapabilities 已声明。
func RegisterSystemTarget(t target.Target) {
	ensureSystemKey("target", t.Name())
	k := t.Name() + "/" + t.DeviceID()
	if _, ok := targets[k]; !ok {
		order = append(order, "target:"+t.Name())
	}
	targets[k] = t
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
// 系统级（Go 内置）词源的 key 受保护：命中则拒绝覆盖并记日志。
func RegisterLyric(l lyric.Provider) {
	if IsBuiltinKey("lyric", l.Name()) {
		log.Printf("[plugin] 拒绝覆盖系统级词源 %q", l.Name())
		return
	}
	lyrics[l.Name()] = l
}

// RegisterSystemLyric 注册系统级（Go 内置）词源：key 必须在 systemCapabilities 已声明。
func RegisterSystemLyric(l lyric.Provider) {
	ensureSystemKey("lyric", l.Name())
	lyrics[l.Name()] = l
}

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
