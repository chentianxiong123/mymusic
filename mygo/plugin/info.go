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

// InfoNames 指定类型的插件名列表（插件页分组）。
func InfoNames(typeName string) []Info {
	var out []Info
	for _, v := range infos {
		if v.Type == typeName {
			out = append(out, v)
		}
	}
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
