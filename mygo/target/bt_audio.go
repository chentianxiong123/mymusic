package target

import "strings"

// audioUUIDPrefixes 蓝牙音频服务 UUID 前缀（短 UUID，完整是 0000110a-0000-1000-8000-00805f9b34fb）。
// A2DP Source/Sink + HFP/HSP + Headset/AG —— 有这些服务的设备才能作为音频输出。
var audioUUIDPrefixes = []string{
	"0000110a", // Audio Source（手机把音频推给音箱；Nokia 3208c 实测有）
	"0000110b", // Audio Sink（A2DP 耳机/音箱，接收音频）
	"00001108", // Headset
	"00001112", // Headset AG
	"0000111e", // Handsfree
	"0000111f", // Handsfree AG
}

// hasAudioUUID 检查 bluetoothctl/blueutil info 输出里有没有音频服务 UUID。
// 各平台后端共用（Linux 查 bluetoothctl info，macOS 查 blueutil --info）。
func hasAudioUUID(infoOut string) bool {
	l := strings.ToLower(infoOut)
	for _, u := range audioUUIDPrefixes {
		if strings.Contains(l, u) {
			return true
		}
	}
	return false
}

// isAudioName 按设备名关键词判断是不是蓝牙音频设备（Windows 等拿不到 UUID 的平台用）。
// 保守策略：宁可漏掉名字奇怪的耳机，也不让鼠标/键盘出现在"音频到此"列表里。
func isAudioName(name string) bool {
	l := strings.ToLower(name)
	for _, kw := range []string{
		"headset", "headphone", "speaker", "airpod", "buds", "耳机", "音箱", "音响",
	} {
		if strings.Contains(l, kw) {
			return true
		}
	}
	return false
}
