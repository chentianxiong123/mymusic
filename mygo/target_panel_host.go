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

// ── 蓝牙设备操作（走 target.Target 插件表，不硬编码后端）──

func (a *app) findBtTarget(id string) target.Target {
	for _, t := range plugin.Targets() {
		if t.Name() == "bluetooth" && t.DeviceID() == id {
			return t
		}
	}
	return nil
}

func (a *app) ConnectBluetooth(id string) {
	t := a.findBtTarget(id)
	if t == nil {
		return
	}
	if b, ok := t.(*target.Bluetooth); ok {
		err := b.Connect()
		if err != nil {
			a.status = "连接失败：" + err.Error()
		} else {
			a.status = "已连接 " + b.DeviceName()
		}
	}
	a.discoverBluetooth()
	a.win.Invalidate()
}

func (a *app) DisconnectBluetooth(id string) {
	t := a.findBtTarget(id)
	if t == nil {
		return
	}
	if b, ok := t.(*target.Bluetooth); ok {
		_ = b.Disconnect()
		a.status = "已断开 " + b.DeviceName()
	}
	a.discoverBluetooth()
	a.win.Invalidate()
}

func (a *app) PlayBluetooth(id, name string) {
	t := a.findBtTarget(id)
	if t == nil {
		return
	}
	if err := t.Play("", target.PlayOpts{Title: a.CurrentTitle()}); err != nil {
		a.status = "蓝牙投送失败：" + err.Error()
	} else {
		a.pauseLocalForCast()
		a.btOn = name
		a.status = "音频已路由到 " + name
	}
	a.win.Invalidate()
}

func (a *app) StopBluetooth() {
	for _, t := range plugin.Targets() {
		if t.Name() == "bluetooth" {
			_ = t.Stop()
		}
	}
	a.btOn = ""
	a.status = "已恢复本机输出"
	a.win.Invalidate()
}

func (a *app) BtBackendName() string {
	b := target.Bt()
	if b == nil {
		return "当前平台不支持蓝牙输出"
	}
	return "蓝牙后端：" + b.Name()
}

func (a *app) Status() string { return a.status }

func (a *app) ToggleLocal() { a.togglePlay() }

// SwitchToLocal 停止所有远程目标（DLNA/蓝牙/音箱），恢复本机播放。
func (a *app) SwitchToLocal() {
	if a.castOn != "" {
		a.StopCast()
	}
	if a.btOn != "" {
		a.StopBluetooth()
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

func (a *app) Invalidate()         { a.win.Invalidate() }
func (a *app) Resp(a_, b float32) float32 { return a.resp(a_, b) }

// 编译期断言：app 实现 TargetPanelHost。
var _ plugin.TargetPanelHost = (*app)(nil)
