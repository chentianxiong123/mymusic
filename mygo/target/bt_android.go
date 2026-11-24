//go:build android

// bt_android.go —— Android 蓝牙后端：系统自动把音频路由到已连接蓝牙设备，
// APP 无法直接改全局输出（无系统权限）。这里提供只读能力 + Kotlin 桥接扩展点。
package target

import (
	"fmt"
	"sync"
)

type btAndroid struct{}

func init() { RegisterBtBackend(&btAndroid{}) }

func (b *btAndroid) Name() string    { return "android-system" }
func (b *btAndroid) Available() bool { return true }

func (b *btAndroid) List() []BtDevice {
	// Android 侧蓝牙设备由 Kotlin 插件通过 SetBtDevices 注入（下方扩展点）
	mu.Lock()
	defer mu.Unlock()
	out := make([]BtDevice, len(androidDevices))
	copy(out, androidDevices)
	return out
}

func (b *btAndroid) Connect(id string) error {
	return fmt.Errorf("请在系统蓝牙设置中连接设备")
}

func (b *btAndroid) Disconnect(id string) error {
	return fmt.Errorf("请在系统蓝牙设置中断开设备")
}

func (b *btAndroid) RouteTo(id string) error {
	// 系统自动路由：已连接蓝牙设备时 audio.Player 输出自然走蓝牙；
	// 这里只做校验，避免用户以为切换失败。
	for _, d := range b.List() {
		if d.MAC == id && d.Connected {
			return nil
		}
	}
	return fmt.Errorf("设备未连接（系统音频会自动路由到已连接蓝牙设备）")
}

func (b *btAndroid) RouteToLocal() error {
	// 断开蓝牙后系统自动回退本机；无需操作。
	return nil
}

// ── Kotlin 桥接扩展点 ──────────────────────────────────────────────
// 安卓原生插件可调用 target.SetBtDevices 注入设备列表（含连接状态）。
var (
	mu             sync.Mutex
	androidDevices []BtDevice
)

// SetBtDevices 注入安卓侧蓝牙设备列表（Kotlin 通过 bridge 调用）。
func SetBtDevices(devices []BtDevice) {
	mu.Lock()
	androidDevices = append(androidDevices[:0], devices...)
	mu.Unlock()
}
