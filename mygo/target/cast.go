// DLNA 投屏目标插件：包装主线 cast 域（Cast/Control 复用，不重写 DLNA 逻辑）。
package target

import (
	"homecast/service/cast"
)

// CastDevice DLNA 投屏目标（每台设备一个实例）。
// Play = cast.Cast（B站页面 URL → 解析流 → 投屏）；Stop = Control stop。
type CastDevice struct {
	svc  *cast.CastService
	udn  string
	name string
}

// NewCast 创建 DLNA 投屏插件（udn=设备唯一名，name=设备名）。
func NewCast(svc *cast.CastService, udn, name string) *CastDevice {
	return &CastDevice{svc: svc, udn: udn, name: name}
}

func (t *CastDevice) Name() string       { return "cast" }
func (t *CastDevice) DeviceID() string   { return t.udn }
func (t *CastDevice) DeviceName() string { return t.name }

// Play 投屏当前播放（opts.URL=完整地址；缺省用 opts.SongID 拼 B站页面 URL）。
func (t *CastDevice) Play(songID string, opts PlayOpts) error {
	url := opts.URL
	if url == "" {
		url = "https://www.bilibili.com/video/" + songID
	}
	res, err := t.svc.Cast(url, t.udn, opts.Title)
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

// Stop 停止投屏（DLNA Stop 动作）。
func (t *CastDevice) Stop() error {
	_, _ = t.svc.Control(t.udn, "stop", "", 0)
	return nil
}
