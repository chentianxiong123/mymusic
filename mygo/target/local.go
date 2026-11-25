// 本机声卡目标插件：本地播放（MyGoOutput）也抽象成 Target，一切皆插件。
package target

import (
	"homecast/model"
	"homecast/service/playback"
)

// LocalPlayer 本机声卡播放目标（默认播放路径，DeviceID=""）。
// 包装 MyGoOutput.Play/Stop（本机 audio.Player 出声）。
type LocalPlayer struct {
	out interface {
		Play(bvid string, opts playback.PlayOpts) error
		Stop() error
	}
}

// NewLocal 创建本机声卡插件。
func NewLocal(out interface {
	Play(bvid string, opts playback.PlayOpts) error
	Stop() error
}) *LocalPlayer {
	return &LocalPlayer{out: out}
}

func (t *LocalPlayer) Name() string       { return "local" }
func (t *LocalPlayer) DeviceID() string   { return "" }
func (t *LocalPlayer) DeviceName() string { return "本机声卡" }

// Play 本机播放（songID=bvid）。
func (t *LocalPlayer) Play(ref model.SongRef, opts PlayOpts) error {
	return t.out.Play(ref.ID, playback.PlayOpts{Title: opts.Title, Source: ref.Source, ID: ref.ID})
}

// Stop 本机停止。
func (t *LocalPlayer) Stop() error { return t.out.Stop() }
