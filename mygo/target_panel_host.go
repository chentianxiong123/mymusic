//go:build native

package main

import (
	"homecast/service/cast"
	"homecast/xiaoai"
	"mymusic-mygo/plugin"
	"mymusic-mygo/target"

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
func (a *app) BtDevices() []target.BtDevice     { return a.btDevices }
func (a *app) CastStatusMap() map[string]string { return a.castStatus }
func (a *app) SpkDID() string                   { return a.spkDid }
func (a *app) IsPlaying() bool                  { return a.playing }
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
func (a *app) DiscoverBluetooth()      { a.discoverBluetooth() }
func (a *app) RefreshSpeakers()        { a.refreshSpeakers() }
func (a *app) StartSpeakerLogin()      { a.startSpeakerLogin() }
func (a *app) LoginWithToken(t string) { a.loginWithToken(t) }
func (a *app) FillTokenFromClipboard() { a.fillTokenFromClipboard() }

func (a *app) CastTo(udn, name string)        { a.castTo(udn, name) }
func (a *app) StopCast()                      { a.stopCast() }
func (a *app) PlayToSpeaker(did, name string) { a.playToSpeaker(did, name) }
func (a *app) StopSpeaker(did string)         { a.stopSpeaker(did) }
func (a *app) QueryCastStatus(udn string)     { a.queryCastStatus(udn) }

func (a *app) Invalidate()         { a.win.Invalidate() }
func (a *app) Resp(a_, b float32) float32 { return a.resp(a_, b) }

// 编译期断言：app 实现 TargetPanelHost。
var _ plugin.TargetPanelHost = (*app)(nil)
