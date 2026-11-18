// 蓝牙目标插件：发现蓝牙设备（BlueZ bluetoothctl），投送 = 音频路由到蓝牙 sink。
// 纯命令行对接（零新依赖，复用系统 BlueZ/PipeWire），主线无蓝牙域故不依赖主线。
package target

import (
	"os/exec"
	"strings"
)

// BtDevice 蓝牙设备信息（发现结果）。
type BtDevice struct {
	MAC  string // 设备 MAC（如 E4:EC:10:76:0D:83）
	Name string // 设备名
}

// ScanBluetooth 发现蓝牙设备（bluetoothctl devices）。
func ScanBluetooth() []BtDevice {
	out, err := exec.Command("bluetoothctl", "devices").Output()
	if err != nil {
		return nil
	}
	var list []BtDevice
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Device ") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 3 {
			list = append(list, BtDevice{MAC: parts[1], Name: strings.Join(parts[2:], " ")})
		}
	}
	return list
}

// Bluetooth 蓝牙音箱投送目标（每台设备一个实例）。
// Play = 设蓝牙 sink 为默认（音频路由到蓝牙音箱）；Stop = 恢复本机 sink。
type Bluetooth struct {
	mac  string
	name string
}

// NewBluetooth 创建蓝牙目标插件（mac=设备 MAC，name=设备名）。
func NewBluetooth(mac, name string) *Bluetooth {
	return &Bluetooth{mac: mac, name: name}
}

func (t *Bluetooth) Name() string       { return "bluetooth" }
func (t *Bluetooth) DeviceID() string   { return t.mac }
func (t *Bluetooth) DeviceName() string { return t.name }

// sinkForMAC 找设备对应的 PipeWire/PulseAudio sink（bluez_output.<MAC> 前缀）。
func sinkForMAC(mac string) string {
	out, err := exec.Command("pactl", "list", "short", "sinks").Output()
	if err != nil {
		return ""
	}
	key := "bluez_output." + strings.ReplaceAll(mac, ":", "_")
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, key) {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1] // sink 名
			}
		}
	}
	return ""
}

// Play 路由音频到蓝牙音箱（设默认 sink = 蓝牙）。
func (t *Bluetooth) Play(songID string, opts PlayOpts) error {
	sink := sinkForMAC(t.mac)
	if sink == "" {
		return &PlayErr{"蓝牙音箱未连接（无 bluez_output sink）"}
	}
	return exec.Command("pactl", "set-default-sink", sink).Run()
}

// Stop 恢复本机默认 sink（停止投送）。
func (t *Bluetooth) Stop() error {
	return exec.Command("pactl", "set-default-sink", "@DEFAULT_SINK@").Run()
}
