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

// parseConnectedDevices 解析 "bluetoothctl devices Connected" 输出。
// 格式 "Device <MAC> <名字>"，名字可能有空格也可能没有。
func parseConnectedDevices(out string) map[string]bool {
	res := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) >= 3 && f[0] == "Device" {
			res[strings.ToLower(f[1])] = true
		}
	}
	return res
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
		connected = parseConnectedDevices(co)
	}
	var list []BtDevice
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 || f[0] != "Device" {
			continue
		}
		mac := f[1]
		audio := b.isAudioDevice(mac)
		conn := connected[strings.ToLower(mac)]
		list = append(list, BtDevice{
			MAC: mac, Name: strings.Join(f[2:], " "),
			Connected: conn,
			Supported: audio,
			// 只有真有 bluez_output sink 才能投（设备连上但没注册音频端点时不可投）
			Ready: audio && conn && sinkForMAC(mac) != "",
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
	// 1. 默认 sink 切到蓝牙（影响之后新建的音频流，比如切歌后重新建流）
	if err := exec.Command("pactl", "set-default-sink", sink).Run(); err != nil {
		return err
	}
	// 2. 把当前正在播放的流也迁过去。set-default-sink 不会迁移已存在的流，
	//    不迁移的话当前这首歌还是从扬声器出 —— 就是"点了音频到此没反应"。
	out, err := exec.Command("pactl", "list", "short", "sink-inputs").Output()
	if err != nil {
		return nil // 默认 sink 已切，迁流失败不致命
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		if f[1] == sink {
			continue // 已在这个 sink 上
		}
		_ = exec.Command("pactl", "move-sink-input", f[0], sink).Run()
	}
	return nil
}

func (b *btLinux) RouteToLocal() error {
	// 切回本机（内置扬声器）。@DEFAULT_SINK@ 是占位符，不能当 set-default-sink 参数。
	local := localSink()
	if local == "" {
		return fmt.Errorf("找不到本机音频输出")
	}
	if err := exec.Command("pactl", "set-default-sink", local).Run(); err != nil {
		return err
	}
	// 把还在蓝牙 sink 上的流迁回本机
	out, err := exec.Command("pactl", "list", "short", "sink-inputs").Output()
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && strings.HasPrefix(f[1], "bluez_output.") {
			_ = exec.Command("pactl", "move-sink-input", f[0], local).Run()
		}
	}
	return nil
}

// localSink 找非蓝牙的本机 sink（内置扬声器/声卡）。
func localSink() string {
	out, err := exec.Command("pactl", "list", "short", "sinks").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && !strings.HasPrefix(f[1], "bluez_output.") {
			return f[1]
		}
	}
	return ""
}
