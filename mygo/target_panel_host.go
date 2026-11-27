//go:build native

package main

import (
	"homecast/service/cast"
	"homecast/xiaoai"
	"mymusic-mygo/plugin"
)

// ── app 实现 plugin.TargetPanelHost（目标面板只认接口，不认 app 具体类型）──

func (a *app) HasCurrent() bool    { return a.current != nil }
func (a *app) CurrentBVID() string {
	if a.current == nil {
		return ""
	}
	return a.current.bvid
}
func (a *app) CurrentTitle() string {
	if a.current == nil {
		return ""
	}
	return a.current.title
}

func (a *app) CastDevices() []cast.DeviceDTO    { return a.castDevices }
func (a *app) Speakers() []xiaoai.SpeakerDevice { return a.speakers }
func (a *app) CastStatusMap() map[string]string { return a.castStatus }
func (a *app) SpkDID() string                   { return a.spkDid }
func (a *app) IsPlaying() bool                  { return a.playing }

// ActiveOutput 当前激活的输出目标（统一状态源：各面板自己渲染激活态）。
// 优先级：音箱 > DLNA > 本机（与 SwitchToLocal 的清理顺序一致）。
func (a *app) ActiveOutput() (string, string) {
	if a.spkDid != "" {
		for _, d := range a.speakers {
			if d.DID == a.spkDid {
				return "speaker", d.Name
			}
		}
		return "speaker", "小爱音箱"
	}
	if a.castOn != "" {
		return "dlna", a.castOn
	}
	return "local", ""
}
func (a *app) NextBVID() string {
	if a.pb == nil {
		return ""
	}
	return a.pb.NextBVID()
}

func (a *app) SpeakerLoggedIn() bool { return a.svc != nil && a.svc.speakerLoggedIn() }
func (a *app) SpeakerUserID() string {
	if a.svc == nil {
		return ""
	}
	return a.svc.speakerUserID()
}
func (a *app) QRImage() string        { return a.qrImage }
func (a *app) QRStatus() string       { return a.qrStatus }
func (a *app) TokenInputPtr() *string { return &a.tokenInput }

func (a *app) DiscoverCast()           { a.discoverCast() }
func (a *app) RefreshSpeakers()        { a.refreshSpeakers() }
func (a *app) StartSpeakerLogin()      { a.startSpeakerLogin() }
func (a *app) LoginWithToken(t string) { a.loginWithToken(t) }
func (a *app) FillTokenFromClipboard() { a.fillTokenFromClipboard() }

func (a *app) CastTo(udn, name string)        { a.castTo(udn, name) }
func (a *app) StopCast()                      { a.stopCast() }
func (a *app) PlayToSpeaker(did, name string) { a.playToSpeaker(did, name) }
func (a *app) StopSpeaker(did string)         { a.stopSpeaker(did) }
func (a *app) QueryCastStatus(udn string)     { a.queryCastStatus(udn) }

func (a *app) Status() string { return a.status }

func (a *app) ToggleLocal() { a.togglePlay() }

// SwitchToLocal 停止所有远程目标（DLNA/蓝牙/音箱），恢复本机播放。
func (a *app) SwitchToLocal() {
	if a.castOn != "" {
		a.StopCast()
	}
	for _, d := range a.speakers {
		if a.spkDid == d.DID {
			a.StopSpeaker(d.DID)
		}
	}
	if a.localPausedByCast && a.pb != nil {
		a.localPausedByCast = false
		a.pb.Resume()
	}
	a.status = "已切回本机输出"
	a.win.Invalidate()
}

// pauseLocalForCast 投送到远程前暂停本机，避免两边同时出声。
func (a *app) pauseLocalForCast() {
	if a.pb == nil || a.localPausedByCast {
		return
	}
	if a.pb.Snapshot().Playing {
		a.localPausedByCast = true
		a.pb.Pause()
	}
}

func (a *app) Invalidate() {
	if a.win != nil { // 测试环境/初始化前 win 为 nil，异步回调不能崩
		a.win.Invalidate()
	}
}
func (a *app) Resp(a_, b float32) float32 { return a.resp(a_, b) }

// 编译期断言：app 实现 TargetPanelHost。
var _ plugin.TargetPanelHost = (*app)(nil)
