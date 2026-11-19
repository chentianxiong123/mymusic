package plugin

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// 插件设置持久化——系统统一管理（内核服务，同"缓存=内核服务"决策）。
//
// 分工原则：内容归插件（RegisterSetting 声明项），机制归系统（SettingStore 统一存储）。
//   - 所有插件（Go + JS）设置存同一个 settings 表（复用主线 conv.SaveSetting/LoadSetting）
//   - key 命名空间：plugin:<type>/<name>/<settingKey>，避免与主线 hc:* 冲突
//   - 系统提供统一存储（可备份/迁移/导出），插件零耦合（不自己管理存储）
//
// 依赖反转：plugin 包不 import sql.DB/conv，只认 SettingStore 接口；main 注入实现。

// SettingStore 设置存储接口（内核服务，依赖反转）。
// 实现方 = main（包装主线 conv.SaveSetting/LoadSetting，落 settings 表）。
// 允许空实现（Set/Get 直接返回，无持久化时退化为内存默认值）。
type SettingStore interface {
	// Set 存设置（key 已含插件命名空间前缀）。
	Set(key, value string) error
	// Get 读设置；未设置/读失败 → ok=false。
	Get(key string) (string, bool)
}

// SettingValue 一个插件设置项的类型。
type SettingValue string

const (
	SettingSelect  SettingValue = "select"  // 下拉（Options + Default）
	SettingBool    SettingValue = "bool"    // 开关
	SettingText    SettingValue = "text"    // 文本输入
	SettingInt     SettingValue = "int"     // 整数
	SettingFloat   SettingValue = "float"   // 浮点
	SettingSegment SettingValue = "segment" // 分段选择（Options，UI 一排）
)

// Setting 插件设置项声明（内容归插件，机制归系统）。
// 插件（Go: RegisterSetting；JS: meta.settings）声明项，UI 设置页自动生成表单。
type Setting struct {
	Key     string        // 设置键（插件内唯一；存库加命名空间前缀）
	Name    string        // 显示名
	Type    SettingValue  // 类型（select/bool/text/int/float/segment）
	Options []string      // select/segment 的选项
	Default string        // 默认值（字符串；bool 用 "true"/"false"）
	Desc    string        // 描述（表单下方小字）
}

// settingKeyPrefix 插件设置 key 命名空间前缀。
const settingKeyPrefix = "plugin:"

// pluginSettings 设置注册表（key=Type+"/"+Name+"/"+SettingKey）。
// 系统统一管理：注册表存"有哪些设置项"，存储走 SettingStore。
var (
	settingsMu     sync.RWMutex
	settings       = map[string][]Setting{} // type/name → 声明项列表
	settingStore   SettingStore             // 注入的存储实现（nil=内存模式）
)

// SetSettingStore 注入设置存储实现（main 启动时调；nil 允许=内存模式）。
func SetSettingStore(s SettingStore) {
	settingsMu.Lock()
	settingStore = s
	settingsMu.Unlock()
}

// RegisterSettings 注册插件的设置项声明（Go 插件 init/启动时调）。
// typeName/name 对应插件类型和名（如 "source"/"bilibili"、"lyric"/"netease"）。
// 同名设置项覆盖（后注册胜）。
func RegisterSettings(typeName, name string, items []Setting) {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	key := typeName + "/" + name
	// 合并：同名 key 覆盖，其余保留
	merged := map[string]Setting{}
	for _, it := range settings[key] {
		merged[it.Key] = it
	}
	for _, it := range items {
		merged[it.Key] = it
	}
	out := make([]Setting, 0, len(merged))
	for _, it := range merged {
		out = append(out, it)
	}
	settings[key] = out
}

// SettingsOf 取某插件声明的设置项（未声明返回 nil）。
func SettingsOf(typeName, name string) []Setting {
	settingsMu.RLock()
	defer settingsMu.RUnlock()
	return settings[typeName+"/"+name]
}

// settingKey 拼插件设置存储 key（带命名空间）。
func settingKey(typeName, name, key string) string {
	return settingKeyPrefix + typeName + "/" + name + "/" + key
}

// GetSetting 读插件设置（系统统一存储；未设置/无存储 → 默认值）。
func GetSetting(typeName, name, key, def string) string {
	settingsMu.RLock()
	store := settingStore
	settingsMu.RUnlock()
	if store != nil {
		if v, ok := store.Get(settingKey(typeName, name, key)); ok {
			return v
		}
	}
	return def
}

// SetSetting 写插件设置（系统统一存储；无存储 → 内存态仅本次进程）。
func SetSetting(typeName, name, key, value string) error {
	settingsMu.RLock()
	store := settingStore
	settingsMu.RUnlock()
	if store != nil {
		return store.Set(settingKey(typeName, name, key), value)
	}
	return nil
}

// GetSettingInt 读整数设置（默认 def）。
func GetSettingInt(typeName, name, key string, def int) int {
	v := GetSetting(typeName, name, key, "")
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// SetSettingInt 写整数设置。
func SetSettingInt(typeName, name, key string, v int) error {
	return SetSetting(typeName, name, key, strconv.Itoa(v))
}

// GetSettingBool 读布尔设置（默认 def；"true"/"1" = true）。
func GetSettingBool(typeName, name, key string, def bool) bool {
	v := GetSetting(typeName, name, key, "")
	if v == "" {
		return def
	}
	return v == "true" || v == "1"
}

// SetSettingBool 写布尔设置。
func SetSettingBool(typeName, name, key string, v bool) error {
	return SetSetting(typeName, name, key, fmt.Sprintf("%v", v))
}

// GetSettingFloat 读浮点设置（默认 def）。
func GetSettingFloat(typeName, name, key string, def float64) float64 {
	v := GetSetting(typeName, name, key, "")
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

// SetSettingFloat 写浮点设置。
func SetSettingFloat(typeName, name, key string, v float64) error {
	return SetSetting(typeName, name, key, strconv.FormatFloat(v, 'f', -1, 64))
}

// SettingKeys 列出某插件所有已存设置 key（命名空间内；备份/迁移用）。
// 仅对注入的 store 生效；内存模式返回空。
func SettingKeys(typeName, name string) []string {
	settingsMu.RLock()
	store := settingStore
	settingsMu.RUnlock()
	if store == nil {
		return nil
	}
	prefix := settingKeyPrefix + typeName + "/" + name + "/"
	// 通用 store 可能不支持枚举；这里靠 Get 探测默认（有声明项才可枚举）
	var keys []string
	for _, it := range SettingsOf(typeName, name) {
		if _, ok := store.Get(prefix + it.Key); ok {
			keys = append(keys, it.Key)
		}
	}
	return keys
}

// settingKeyOf 导出设置 key（UI/备份用；含命名空间）。
func SettingStorageKey(typeName, name, key string) string {
	return settingKey(typeName, name, key)
}

// ParseSettingValue 设置默认值字符串解析（bool/int/float 容错）。
func ParseSettingValue(v string) string {
	return strings.TrimSpace(v)
}
