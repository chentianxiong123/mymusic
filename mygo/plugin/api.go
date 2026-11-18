// 稳定链路 API：统一抽象入口，供后续拓展开发（多端/插件）复用。
// 链路：搜索 → 选歌 → 取流 → 播放（本机/投送）→ 歌词。
// 不依赖 UI，任何端（桌面/安卓/未来）都基于这套 API 驱动核心。
package plugin

import (
	"mymusic-mygo/source"
)

// Link 播放链路统一 API（稳定契约：后续拓展基于它，不碰 UI）。
// 实现方 = 播放核心（mygo 的 playback 状态机 + 输出目标）。
type Link interface {
	// Search 当前音源搜索（page 从 1 起，size 每页）。
	Search(keyword string, page, size int) ([]source.Song, int, error)
	// Source 当前音源（可切换）。
	Source() source.MusicSource
	// SetSource 切换音源（网易云/本地/插件）。
	SetSource(name string) bool
	// Sources 可用音源名（含插件注册）。
	Sources() []string

	// Play 播放歌曲（songID=音源 id；target=目标插件名，""=本机声卡）。
	Play(songID, title string, target string) error
	// Pause / Resume / Stop 播放控制。
	Pause() error
	Resume() error
	Stop() error
	// Seek 跳转（秒）。
	Seek(sec float64) error
	// Position 当前进度（秒）。
	Position() float64
	// Targets 可用输出目标（本机/音箱/DLNA/插件）。
	Targets() []TargetBrief

	// Lyric 当前歌歌词（走歌词插件，keywords=备选标题）。
	Lyric(keywords []string, sid int) ([]source.LyricLine, error)
}

// TargetBrief 目标摘要（UI 列表展示用，不含播放句柄）。
type TargetBrief struct {
	Name string // "local"/"speaker"/"cast"
	ID   string // 设备 id（本机=""）
	Desc string // 展示名
}
