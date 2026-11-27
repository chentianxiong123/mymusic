// Package plugin 目标页派生：目标页完全由插件系统派生。
//
// 每个目标种类（DLNA/蓝牙/小爱音箱/未来第三方）注册一个 TargetPanel，
// 目标页只是枚举渲染各面板。渲染回调只认 TargetPanelHost 接口
// （依赖反转：面板不依赖具体 app，未来第三方插件可独立实现/复用）。
package plugin

import (
	"homecast/service/cast"
	"homecast/xiaoai"
	"mymusic-mygo/target"

	"github.com/egoist/mygo/ui"
)

// TargetPanelHost 目标页宿主能力（由主程序 app 实现，注入面板渲染）。
// 面板只通过这个接口访问 app，不认具体类型（第三方插件可复用）。
type TargetPanelHost interface {
	// 当前播放
	HasCurrent() bool
	CurrentBVID() string
	CurrentTitle() string

	// 当前激活输出（kind=local/dlna/bluetooth/speaker，name=设备名；空=本机）
	// 各输出面板据此渲染自己的激活态（统一性：状态归插件自己，不写死在页面里）
	ActiveOutput() (kind, name string)

	// 设备列表
	CastDevices() []cast.DeviceDTO
	Speakers() []xiaoai.SpeakerDevice
	BtDevices() []target.BtDevice
	CastStatusMap() map[string]string
	SpkDID() string
	IsPlaying() bool
	NextBVID() string

	// 音箱登录态
	SpeakerLoggedIn() bool
	SpeakerUserID() string
	QRImage() string
	QRStatus() string
	TokenInputPtr() *string

	// 发现 / 刷新
	DiscoverCast()
	DiscoverBluetooth()
	RefreshSpeakers()
	StartSpeakerLogin()
	LoginWithToken(tok string)
	FillTokenFromClipboard()

	// 投送 / 停止 / 查询
	CastTo(udn, name string)
	StopCast()
	PlayToSpeaker(did, name string)
	StopSpeaker(did string)
	QueryCastStatus(udn string)

	// 蓝牙设备操作
	ConnectBluetooth(id string)
	DisconnectBluetooth(id string)
	PlayBluetooth(id, name string)
	StopBluetooth()
	BtBackendName() string

	// 本机输出
	SwitchToLocal()
	ToggleLocal()

	// UI 辅助
	Invalidate()
	Resp(a, b float32) float32
	Status() string
}

// TargetPanel 一个目标种类的页面区块（DLNA/蓝牙/音箱/...）。
type TargetPanel struct {
	Kind  string // 目标种类 id（"dlna"/"bluetooth"/"speaker"/...）
	Order int    // 页面内排序（小在前）
	Label string // 区块标题
	Icon  string // 图标标识（UI 层映射 SVG）
	Render func(c *ui.Context, h TargetPanelHost) // 渲染区块（含发现/列表/投送）
}

// targetPanels 目标面板注册表（按 Order 排序展示）。
var targetPanels []TargetPanel

// RegisterTargetPanel 注册目标面板（init 里调，或启动时）。同名覆盖。
func RegisterTargetPanel(p TargetPanel) {
	for i, x := range targetPanels {
		if x.Kind == p.Kind {
			targetPanels[i] = p
			return
		}
	}
	targetPanels = append(targetPanels, p)
}

// TargetPanels 已注册目标面板（按 Order 排序）。
func TargetPanels() []TargetPanel {
	out := make([]TargetPanel, len(targetPanels))
	copy(out, targetPanels)
	// 插入排序按 Order（面板少，够用）
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Order < out[j-1].Order; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
