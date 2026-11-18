// 小爱音箱目标插件：包装主线 speaker 域（Play/Stop 复用，不重写音箱逻辑）。
package target

import (
	"homecast/bilibili"
	"homecast/service/playback"
	"homecast/service/speaker"
)

// XiaoaiSpeaker 小爱音箱投送目标（每台设备一个实例）。
// 复用主线 speaker.SpeakerService.Play（DASH → 代理 URL → 音箱）。
type XiaoaiSpeaker struct {
	svc  *speaker.SpeakerService
	pb   *playback.PlaybackState
	did  string
	name string
}

// NewXiaoai 创建小爱音箱目标（did=设备id，name=设备名）。
func NewXiaoai(svc *speaker.SpeakerService, pb *playback.PlaybackState, did, name string) *XiaoaiSpeaker {
	return &XiaoaiSpeaker{svc: svc, pb: pb, did: did, name: name}
}

func (t *XiaoaiSpeaker) Name() string       { return "speaker" }
func (t *XiaoaiSpeaker) DeviceID() string   { return t.did }
func (t *XiaoaiSpeaker) DeviceName() string { return t.name }

// Play 投送当前播放到小爱音箱（音质档映射 B站 qn；NextID 自动连播）。
func (t *XiaoaiSpeaker) Play(songID string, opts PlayOpts) error {
	qn := bilibili.Audio192K
	// 音质档由调用方通过 PlayOpts 传入？—— 主线固定 192k，保持现状
	po := speaker.PlayOpts{Title: opts.Title, Duration: 0, NextBVID: opts.NextID}
	res, err := t.svc.Play(songID, t.did, qn, po)
	if err != nil {
		return err
	}
	if code, _ := res["code"].(int); code != 0 {
		if msg, _ := res["message"].(string); msg != "" {
			return &PlayErr{msg}
		}
	}
	return nil
}

// Stop 停止音箱投送（主线 StopSpeaker 唯一实现）。
func (t *XiaoaiSpeaker) Stop() error {
	// 停止音箱投送：playback.StopSpeaker 唯一实现（音箱停 + source 切回 web）
	t.pb.StopSpeaker(t.did)
	return nil
}

// PlayErr 音箱播放业务错误（code!=0）。
type PlayErr struct{ msg string }

func (e *PlayErr) Error() string { return e.msg }
