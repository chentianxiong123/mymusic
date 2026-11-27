//go:build windows

// bt_windows.go —— Windows 蓝牙后端：PowerShell 读 PnP 蓝牙设备 + SoundVolumeView 路由默认输出。
// SoundVolumeView 是 NirSoft 免费工具（放 PATH 即可）；缺失时 Available=false。
package target

import (
	"fmt"
	"os/exec"
	"strings"
)

type btWindows struct{}

func init() { RegisterBtBackend(&btWindows{}) }

func (b *btWindows) Name() string { return "windows-pnp" }
func (b *btWindows) Available() bool {
	if _, err := exec.LookPath("powershell"); err != nil {
		return false
	}
	_, err := exec.LookPath("SoundVolumeView")
	return err == nil
}

func (b *btWindows) ps(script string) (string, error) {
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// List 已配对蓝牙设备（PnP 设备类 Bluetooth，状态字段区分连接）。
func (b *btWindows) List() []BtDevice {
	script := `Get-PnpDevice -Class Bluetooth | Where-Object { $_.FriendlyName } | ForEach-Object { "$($_.InstanceId)|$($_.FriendlyName)|$($_.Status)" }`
	out, err := b.ps(script)
	if err != nil {
		return nil
	}
	var list []BtDevice
	for _, line := range strings.Split(out, "\n") {
		f := strings.SplitN(strings.TrimSpace(line), "|", 3)
		if len(f) < 3 || f[0] == "" {
			continue
		}
		list = append(list, BtDevice{
			MAC: f[0], Name: f[1],
			Connected: strings.EqualFold(f[2], "OK"),
			Supported: isAudioName(f[1]), // Windows PnP 无 UUID API：按名称关键词判断音频设备
		})
	}
	return list
}

// Connect Windows 无官方 CLI 连蓝牙（设置里连）；这里给出可操作提示。
func (b *btWindows) Connect(id string) error {
	return fmt.Errorf("请在 Windows 设置 → 蓝牙和设备 中连接该设备")
}

func (b *btWindows) Disconnect(id string) error {
	_, err := b.ps(fmt.Sprintf(`Get-PnpDevice -InstanceId '%s' | Disable-PnpDevice -Confirm:$false`, id))
	return err
}

// RouteTo SoundVolumeView /SetDefault "<设备名>" Render
func (b *btWindows) RouteTo(id string) error {
	name := id
	for _, d := range b.List() {
		if d.MAC == id {
			name = d.Name
			break
		}
	}
	out, err := exec.Command("SoundVolumeView", "/SetDefault", name, "Render").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
}

// RouteToLocal 切回系统默认渲染设备（先清 Bluetooth 默认，再由系统回退）。
func (b *btWindows) RouteToLocal() error {
	out, err := exec.Command("SoundVolumeView", "/SetDefault", "Speakers", "Render").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
}
