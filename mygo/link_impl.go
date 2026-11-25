//go:build native

package main

import (
	"errors"
	"strings"

	hmodel "homecast/model"
	"homecast/service/playback"
	"mymusic-mygo/plugin"
	"mymusic-mygo/model"
	"mymusic-mygo/source"
	"mymusic-mygo/target"
)

// linkCore 实现 plugin.Link（稳定链路 API：搜索→播放→控制→歌词的统一入口）。
//
// 设计：不依赖 UI 渲染，只持可复用核心（playback 状态机 + 音源/目标/歌词注册表），
// 任何端（桌面/安卓/WASM）都能独立构造并基于它驱动核心，不碰 UI。
type linkCore struct {
	pb *playback.PlaybackState // 统一播放状态机（主线域包，可复用）
}

// NewLink 创建稳定链路实现（注入播放核心）。
func NewLink(pb *playback.PlaybackState) *linkCore {
	return &linkCore{pb: pb}
}

// 编译期断言：linkCore 实现 plugin.Link。
var _ plugin.Link = (*linkCore)(nil)

// ── 音源 ──

func (l *linkCore) Sources() []string { return plugin.SourceNames() }

func (l *linkCore) Source() source.MusicSource {
	// 当前音源 = 注册表第一个（B站）；未来可持久化选择
	if names := plugin.SourceNames(); len(names) > 0 {
		return plugin.Sources()[names[0]]
	}
	return nil
}

func (l *linkCore) SetSource(name string) bool {
	if _, ok := plugin.Sources()[name]; !ok {
		return false
	}
	// 音源切换：注册表本身无状态，切换由调用方持久化（UI 存 pref）
	return true
}

func (l *linkCore) Search(keyword string, page, size int) ([]source.Song, int, error) {
	src := l.Source()
	if src == nil {
		return nil, 0, errors.New("无可用音源")
	}
	return src.Search(keyword, page, size)
}

// ── 播放 ──

func (l *linkCore) Play(songID, title, targetName string) error {
	// 投送目标：非空且非本机 → 找对应 target 插件
	if targetName != "" && targetName != "local" {
		t := findTarget(targetName)
		if t == nil {
			return errors.New("目标未注册: " + targetName)
		}
		return t.Play(hmodel.SongRef{Source: "bilibili", ID: songID}, target.PlayOpts{Title: title})
	}
	// 本机：走统一播放状态机（自动调度输出）
	if l.pb == nil {
		return errors.New("播放核心未初始化")
	}
	l.pb.Play(playback.Song{BVID: songID, Title: title})
	return nil
}

func (l *linkCore) Pause() error {
	if l.pb == nil {
		return errors.New("播放核心未初始化")
	}
	l.pb.Pause()
	return nil
}

func (l *linkCore) Resume() error {
	if l.pb == nil {
		return errors.New("播放核心未初始化")
	}
	l.pb.Resume()
	return nil
}

func (l *linkCore) Stop() error {
	if l.pb == nil {
		return errors.New("播放核心未初始化")
	}
	l.pb.Pause() // 统一暂停（音箱走真 Pause 停声；web 前端 audio 接管）
	return nil
}

func (l *linkCore) Seek(sec float64) error {
	if l.pb == nil {
		return errors.New("播放核心未初始化")
	}
	dur := l.pb.Snapshot().Duration
	if dur <= 0 {
		return errors.New("时长未知，不可 seek")
	}
	if sec < 0 {
		sec = 0
	}
	if sec > dur {
		sec = dur
	}
	l.pb.Seek(sec)
	return nil
}

func (l *linkCore) Position() float64 {
	if l.pb == nil {
		return 0
	}
	return l.pb.Snapshot().Position
}

// ── 目标 ──

func (l *linkCore) Targets() []plugin.TargetBrief {
	var out []plugin.TargetBrief
	for _, t := range plugin.Targets() {
		out = append(out, plugin.TargetBrief{
			Name: t.Name(),
			ID:   t.DeviceID(),
			Desc: t.DeviceName(),
		})
	}
	return out
}

// ── 歌词 ──

func (l *linkCore) Lyric(keywords []string, sid string) ([]model.LyricLine, error) {
	prov := plugin.Lyric("") // 默认网易云
	if prov == nil {
		return nil, errors.New("无歌词插件")
	}
	return prov.Fetch(keywords, sid) // sid string（lyric.Provider 签名 string；咪咕 contentId 超 JS 安全整数须字符串保精度）
}

// findTarget 按 "name" 或 "name/id" 或 "id" 找已注册目标插件。
func findTarget(sel string) target.Target {
	for _, t := range plugin.Targets() {
		// 精确 name 或 name/deviceID
		if sel == t.Name() || sel == t.Name()+"/"+t.DeviceID() {
			return t
		}
		// 按设备 id 匹配（sel 可能是设备 id）
		if sel == t.DeviceID() && t.DeviceID() != "" {
			return t
		}
	}
	// 容错：去掉空白/前缀
	sel = strings.TrimSpace(sel)
	for _, t := range plugin.Targets() {
		if strings.EqualFold(sel, t.DeviceName()) {
			return t
		}
	}
	return nil
}
