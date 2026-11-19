//go:build native

package main

import (
	"testing"

	"homecast/service/playback"
	"mymusic-mygo/plugin"
	"mymusic-mygo/source"
	"mymusic-mygo/model"
	"mymusic-mygo/target"
)

// TestLinkImplements 编译期契约：linkCore 实现 plugin.Link（稳定链路 API）。
func TestLinkImplements(t *testing.T) {
	var _ plugin.Link = (*linkCore)(nil) // 编译期断言（重复声明，测试独立）
	l := NewLink(playback.NewPlaybackState())
	if l == nil {
		t.Fatal("NewLink 返回 nil")
	}
}

// fakeSource 假音源（测试注册用，不依赖 app 初始化）。
type fakeSource struct{}

func (fakeSource) Name() string { return "fake" }
func (fakeSource) Search(kw string, page, size int) ([]source.Song, int, error) {
	return []source.Song{{ID: "s1", Title: "歌1", Typename: "fake"}}, 1, nil
}
func (fakeSource) GetStream(id string, q int) (*source.Stream, error) { return &source.Stream{URL: "http://x/a.mp3"}, nil }
func (fakeSource) GetLyric(id string, kw []string, sid int) ([]model.LyricLine, error) {
	return []model.LyricLine{{Time: 1, Text: "词"}}, nil
}

// fakeTarget 假目标（测试注册用）。
type fakeTarget struct{}

func (fakeTarget) Name() string       { return "fakeT" }
func (fakeTarget) DeviceID() string   { return "dev1" }
func (fakeTarget) DeviceName() string { return "假目标" }
func (fakeTarget) Play(songID string, opts target.PlayOpts) error { return nil }
func (fakeTarget) Stop() error        { return nil }

// TestLinkSources 音源列表 + 当前源（注册 fake 后非空）。
func TestLinkSources(t *testing.T) {
	plugin.RegisterSource(fakeSource{})
	l := NewLink(playback.NewPlaybackState())
	if len(l.Sources()) == 0 {
		t.Fatal("应至少有一个音源（fake 已注册）")
	}
	if l.Source() == nil {
		t.Fatal("当前音源不应为 nil")
	}
	// Search 走当前源
	songs, total, err := l.Search("kw", 1, 20)
	if err != nil || len(songs) != 1 || total != 1 {
		t.Fatalf("Search 结果异常: n=%d total=%d err=%v", len(songs), total, err)
	}
	// SetSource 未知源返回 false
	if l.SetSource("no-such") {
		t.Fatal("未知源应返回 false")
	}
}

// TestLinkPlayControl 播放控制状态机（无输出，走 NullOutput 不崩）。
func TestLinkPlayControl(t *testing.T) {
	l := NewLink(playback.NewPlaybackState())
	if err := l.Play("test-bvid", "测试歌", ""); err != nil {
		t.Fatalf("Play 本机失败: %v", err)
	}
	// 无输出（NullOutput）时 Pause/Resume/Seek 应无错误
	if err := l.Pause(); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if err := l.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if err := l.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	// Seek 时长未知应报错（不 panic）
	_ = l.Seek(10)
}

// TestLinkTargets 目标列表（注册 fake 目标后非空）。
func TestLinkTargets(t *testing.T) {
	plugin.RegisterTarget(fakeTarget{})
	l := NewLink(playback.NewPlaybackState())
	targets := l.Targets()
	if len(targets) == 0 {
		t.Fatal("应至少有目标（fake 已注册）")
	}
	// 投送未知目标报错
	if err := l.Play("s1", "歌1", "no-such-target"); err == nil {
		t.Fatal("未知目标应报错")
	}
}
