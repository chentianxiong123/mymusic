package plugin

// 系统级（Go 内置）能力清单 —— 唯一事实来源。
//
// 「系统级插件」= 之前用 Go 写的那些能力（B站源/本机源/网易云词源/本机声卡/投屏/蓝牙/小爱）。
// 它们的 key 受保护：JS 插件与第三方插件不可占用，否则会静默覆盖系统兜底。
//
// 设计取舍（重要）：
//   - **静态显式声明，而非从注册行为派生**。派生式有致命时序依赖——系统注册发生在
//     main.go 启动流程，包级代码/测试未跑它就无从受保护（见 TestJSPluginLyricConflict）。
//   - 代价是清单要人工维护，但漂移会被**注册期校验**当场拦下：
//     新增 Go 内置能力却忘了在此登记 → RegisterSystem* 直接 panic，不会静默漏保护。

import (
	"fmt"
	"sort"
)

// systemCapabilities 系统级能力清单：capability 类型 → 受保护 key 列表。
// 新增系统级能力：先在此声明，再用 RegisterSystem* 注册（漏登记即 panic）。
var systemCapabilities = map[string][]string{
	"source": {"bilibili", "local"},                     // Go 音源
	"lyric":  {"netease"},                               // Go 词源（兜底匹配）
	"target": {"local", "cast", "bluetooth", "speaker"}, // Go 播放目标
}

// IsBuiltinKey 该 (kind,key) 是否为系统级能力（JS 插件不可占用）。
func IsBuiltinKey(kind, key string) bool {
	for _, k := range systemCapabilities[kind] {
		if k == key {
			return true
		}
	}
	return false
}

// CheckPluginKey 校验第三方（JS）插件 key 是否占用系统级 key。
// 供 JS 插件加载期 validate 使用：命中则整个插件加载失败（干净报错，不留半注册状态）。
func CheckPluginKey(kind, key string) error {
	if IsBuiltinKey(kind, key) {
		return fmt.Errorf("key %q 与系统级 %s 能力冲突（Go 内置，JS 插件不可覆盖）", key, kind)
	}
	return nil
}

// ensureSystemKey 校验系统级注册确实属于已声明清单 —— 防"新增 Go 能力却忘了登记"的静默漂移。
func ensureSystemKey(kind, key string) {
	if !IsBuiltinKey(kind, key) {
		panic(fmt.Sprintf(
			"plugin: %s 能力 %q 未在 systemCapabilities 声明；系统级能力须先在 plugin/builtin.go 登记",
			kind, key))
	}
}

// SystemCapabilityKeys 按类型列出系统级 key（插件页/调试用）。
func SystemCapabilityKeys() map[string][]string {
	out := make(map[string][]string, len(systemCapabilities))
	for kind, keys := range systemCapabilities {
		names := append([]string(nil), keys...)
		sort.Strings(names)
		out[kind] = names
	}
	return out
}
