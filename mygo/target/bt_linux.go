//go:build linux && !android

// bt_linux.go —— Linux 蓝牙后端：BlueZ bluetoothctl（配对/连接）+ PipeWire/Pulse pactl（音频路由）。
package target

import (
	"fmt"
	"os/exec"
	"strings"
)

type btLinux struct{}

func init() { RegisterBtBackend(&btLinux{}) }

func (b *btLinux) Name() string { return "linux-bluez" }
func (b *btLinux) Available() bool {
	_, e1 := exec.LookPath("bluetoothctl")
	_, e2 := exec.LookPath("pactl")
	return e1 == nil && e2 == nil
}

func (b *btLinux) run(args ...string) (string, error) {
	out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %v", strings.TrimSpace(string(out)), err)
	}
	return string(out), nil
}

// isAudioDevice 查 bluetoothctl info 的 UUID 判断是不是音频设备。
// 查询失败时保守返回 false（宁可不显示，也不让用户点了没反应）。
func (b *btLinux) isAudioDevice(mac string) bool {
	out, err := b.run("bluetoothctl", "info", mac)
	if err != nil {
		return false
	}
	return hasAudioUUID(out)
}

// List 已配对设备（bluetoothctl devices；只保留音频设备）。
func (b *btLinux) List() []BtDevice {
	// paired-devices 在 bluez 5.82+ 已移除，直接用 devices（列已知/已配对设备）
	out, err := b.run("bluetoothctl", "devices")
	if err != nil {
		return nil
	}
	// 连接集合（info 输出 Connected: yes）
	connected := map[string]bool{}
	if co, e := b.run("bluetoothctl", "devices", "Connected"); e == nil {
		for _, line := range strings.Split(co, "\n") {
			f := strings.Fields(line)
			if len(f) >= 4 && f[0] == "Device" {
				connected[strings.ToLower(f[1])] = true
			}
		}
	}
	var list []BtDevice
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 || f[0] != "Device" {
			continue
		}
		mac := f[1]
		list = append(list, BtDevice{
			MAC: mac, Name: strings.Join(f[2:], " "),
			Connected: connected[strings.ToLower(mac)],
			Supported: b.isAudioDevice(mac), // 音频设备过滤（鼠标/键盘不显示投送）
		})
	}
	return list
}

func (b *btLinux) Connect(mac string) error {
	_, err := b.run("bluetoothctl", "connect", mac)
	return err
}

func (b *btLinux) Disconnect(mac string) error {
	_, err := b.run("bluetoothctl", "disconnect", mac)
	return err
}

// sinkForMAC 找设备对应的 PipeWire/Pulse sink（bluez_output.<MAC> 前缀）。
func sinkForMAC(mac string) string {
	out, err := exec.Command("pactl", "list", "short", "sinks").Output()
	if err != nil {
		return ""
	}
	key := "bluez_output." + strings.ReplaceAll(mac, ":", "_")
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, key) {
			f := strings.Fields(line)
			if len(f) >= 2 {
				return f[1]
			}
		}
	}
	return ""
}

func (b *btLinux) RouteTo(id string) error {
	sink := sinkForMAC(id)
	if sink == "" {
		return fmt.Errorf("蓝牙设备未连接（无 bluez_output sink）")
	}
	return exec.Command("pactl", "set-default-sink", sink).Run()
}

func (b *btLinux) RouteToLocal() error {
	return exec.Command("pactl", "set-default-sink", "@DEFAULT_SINK@").Run()
}
