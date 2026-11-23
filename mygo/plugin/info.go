// 插件元数据：插件页展示用（名称/类型/图标/版本/描述）。
// 用 Info 结构 + 注册表存，不要求插件 import plugin（避免子包循环依赖）。
package plugin

// Info 插件元数据（插件页列表展示）。
type Info struct {
	Name    string // 插件名（bilibili/netease/speaker/cast/bluetooth/local）
	Type    string // 类型（source/lyric/target）
	Icon    string // 图标标识
	Version string // 版本
	Desc    string // 描述
	Order   int    // 展示顺序（小在前；固定，防 map 遍历随机导致条目跳动）
}

// infos 插件元数据表（key=Type+"/"+Name）。
var infos = map[string]Info{}

// RegisterInfo 注册插件元数据（启动时填；插件页读取展示）。
func RegisterInfo(i Info) { infos[i.Type+"/"+i.Name] = i }

// InfoOf 取指定类型+名的元数据（未注册返回空 Info）。
func InfoOf(typeName, name string) Info { return infos[typeName+"/"+name] }

// AllInfos 全部插件元数据（插件页列表）。
func AllInfos() []Info {
	var out []Info
	for _, v := range infos {
		out = append(out, v)
	}
	return out
}

// orderOverride 用户自定义顺序（key=Type/Name → 顺序号，小在前）。
// 用户在插件页调整顺序后写入，影响插件列表/搜索下拉框/所有表征顺序。
var orderOverride = map[string]int{}

// SetOrderOverride 设置用户自定义顺序（从 settings 读回 / 用户调整后写回）。
func SetOrderOverride(m map[string]int) {
	orderOverride = m
}

// OrderOverride 返回当前顺序覆盖表（供 UI 持久化）。
func OrderOverride() map[string]int { return orderOverride }

// sortInfos 排序：先按用户覆盖顺序（有值的优先），再按插件 meta.Order。
// 稳定排序保证未覆盖的插件保持原相对顺序。
func sortInfos(list []Info) {
	rank := func(i Info) (int, int) {
		if v, ok := orderOverride[i.Type+"/"+i.Name]; ok {
			return 0, v // 用户排序优先
		}
		return 1, i.Order // 未覆盖：按 meta.Order
	}
	less := func(a, b Info) bool {
		ag, as := rank(a)
		bg, bs := rank(b)
		return ag < bg || (ag == bg && as < bs)
	}
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && less(list[j], list[j-1]); j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
}

// InfoNames 指定类型的插件名列表（插件页分组，按用户顺序/meta.Order）。
func InfoNames(typeName string) []Info {
	var out []Info
	for _, v := range infos {
		if v.Type == typeName {
			out = append(out, v)
		}
	}
	sortInfos(out)
	return out
}

// --- 插件启用/禁用（插件设置核心：开关决定插件是否生效） ---

// enabled 插件启用状态（key=Type+"/"+Name，默认启用）。
var enabled = map[string]bool{}

// SetEnabled 启用/禁用插件（持久化由 UI 存 settings kv）。
func SetEnabled(typeName, name string, on bool) {
	enabled[typeName+"/"+name] = on
}

// IsEnabled 插件是否启用（未设置=启用）。
func IsEnabled(typeName, name string) bool {
	if v, ok := enabled[typeName+"/"+name]; ok {
		return v
	}
	return true
}

// SourceNames 已注册音源名（按注册顺序）——原注册表方法，加启
// 注意：此方法已在 plugin.go 定义，这里不重复。
