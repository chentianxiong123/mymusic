//go:build darwin

// bt_darwin.go —— macOS 蓝牙后端：blueutil（配对/连接）+ SwitchAudioSource（音频路由）。
// 两个工具都非系统自带（Homebrew 可装）；缺失时 Available=false，UI 优雅降级。
package target

import (
	"fmt"
	"os/exec"
	"strings"
)

type btDarwin struct{}

func init() { RegisterBtBackend(&btDarwin{}) }

func (b *btDarwin) Name() string { return "macos-blueutil" }
func (b *btDarwin) Available() bool {
	_, e1 := exec.LookPath("blueutil")
	_, e2 := exec.LookPath("SwitchAudioSource")
	return e1 == nil && e2 == nil
}

func (b *btDarwin) run(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %v", strings.TrimSpace(string(out)), err)
	}
	return string(out), nil
}

// List blueutil --paired 输出 "address, name ("Friendly")" 或 "address, name"。
func (b *btDarwin) List() []BtDevice {
	out, err := b.run("blueutil", "--paired")
	if err != nil {
		return nil
	}
	// 已连接地址集合
	connected := map[string]bool{}
	if co, e := b.run("blueutil", "--connected"); e == nil {
		for _, line := range strings.Split(co, "\n") {
			a := strings.TrimSpace(strings.SplitN(line, ",", 2)[0])
			if a != "" {
				connected[strings.ToLower(a)] = true
			}
		}
	}
	var list []BtDevice
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ",", 2)
		if len(parts) < 2 {
			continue
		}
		addr := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])
		// 去掉尾部 ("Name") 保留设备名
		if i := strings.Index(name, " ("); i > 0 {
			name = name[:i]
		}
		list = append(list, BtDevice{
			MAC: addr, Name: name,
			Connected: connected[strings.ToLower(addr)], Supported: true,
		})
	}
	return list
}

func (b *btDarwin) Connect(id string) error    { _, err := b.run("blueutil", "--connect", id); return err }
func (b *btDarwin) Disconnect(id string) error { _, err := b.run("blueutil", "--disconnect", id); return err }

// RouteTo 按设备名找音频输出设备（macOS 蓝牙音频设备名通常与 blueutil 名一致）。
func (b *btDarwin) RouteTo(id string) error {
	// 先按 id（地址）找，找不到用设备名
	name := id
	for _, d := range b.List() {
		if strings.EqualFold(d.MAC, id) {
			name = d.Name
			break
		}
	}
	_, err := b.run("SwitchAudioSource", "-s", name)
	return err
}

func (b *btDarwin) RouteToLocal() error {
	// 切回内置输出（macOS 常见名 "MacBook Pro 扬声器"/"内置扬声器"；列出后取第一个内置）
	out, err := b.run("SwitchAudioSource", "-a")
	if err == nil {
		for _, line := range strings.Split(out, "\n") {
			n := strings.TrimSpace(line)
			if strings.Contains(n, "内置") || strings.Contains(n, "Built-in") || strings.Contains(n, "Internal") {
				_, err = b.run("SwitchAudioSource", "-s", n)
				return err
			}
		}
	}
	// 回退：切到 "System Default" 不算，直接返回错误让 UI 提示
	return fmt.Errorf("未找到内置输出设备（请手动切回）")
}
