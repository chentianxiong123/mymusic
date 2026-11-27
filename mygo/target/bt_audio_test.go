//go:build native && linux && !android

// bt_audio_test.go —— 蓝牙音频设备过滤测试（纯逻辑，不碰硬件）。
package target

import "testing"

// Linux 实测：Nokia 3208c 有 Audio Source (110a) + Headset AG (1112)，
// BT5.2 Mouse 只有 HID/Battery。UUID 检测必须区分二者。
func TestAudioUUIDDetection(t *testing.T) {
	nokia := `Name: Nokia 3208c
	UUID: Audio Source (0000110a-0000-1000-8000-00805f9b34fb)
	UUID: Headset AG (00001112-0000-1000-8000-00805f9b34fb)`
	mouse := `Name: BT5.2 Mouse
	UUID: Human Interface Device (00001812-0000-1000-8000-00805f9b34fb)
	UUID: Battery Service (0000180f-0000-1000-8000-00805f9b34fb)`
	if !hasAudioUUID(nokia) {
		t.Fatal("Nokia 3208c（有 Audio Source UUID）应识别为音频设备")
	}
	if hasAudioUUID(mouse) {
		t.Fatal("BT5.2 Mouse（只有 HID/Battery）不应识别为音频设备")
	}
}

// bluetoothctl devices Connected 输出的名字可能有空格也可能没有，
// 解析只看 f[1]（MAC），len>=3 就够。回归：小爱音箱-5846 无空格被误判未连接。
func TestParseConnectedDevices(t *testing.T) {
	out := `Device 40:31:3C:63:5D:FC 小爱音箱-5846
Device 13:05:AA:00:0D:DD BT5.2 Mouse
`
	got := parseConnectedDevices(out)
	if !got["40:31:3c:63:5d:fc"] {
		t.Fatal("无空格名字的小爱音箱应识别为已连接")
	}
	if !got["13:05:aa:00:0d:dd"] {
		t.Fatal("有空格名字的鼠标应识别为已连接")
	}
}

func TestIsAudioName(t *testing.T) {
	for _, name := range []string{"AirPods Pro", "JBL Speaker", "小米耳机", "WH-1000XM5 Headphones"} {
		if !isAudioName(name) {
			t.Errorf("%q 应识别为音频设备", name)
		}
	}
	for _, name := range []string{"BT5.2 Mouse", "Logitech Keyboard", "Nokia 3208c"} {
		if isAudioName(name) {
			t.Errorf("%q 不应识别为音频设备", name)
		}
	}
}
