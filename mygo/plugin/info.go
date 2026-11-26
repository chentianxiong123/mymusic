// 插件元数据：一体化模型（身份 = 平台 key；能力 = 插件属性）。
// 同一个平台只注册一个 Info，Capabilities 声明它提供哪些能力（source/lyric/recommend/target）。
package plugin

// Info 插件元数据（插件页列表展示）。
type Info struct {
	Name         string   // 平台 key（唯一身份：bilibili/netease/kugou/...）
	DisplayName  string   // 展示名（空则用 Name）
	Type         string   // 主能力（兼容：source/lyric/target）—— 新代码用 Capabilities
	Capabilities []string // 能力列表（source/lyric/recommend/target）
	Icon         string
	Version      string
	Desc         string
	Order        int
}

// infos 插件元数据表（key=平台 Name）。同平台重复注册 → 合并能力。
var infos = map[string]Info{}

// RegisterInfo 注册插件元数据：同平台合并 Capabilities（去重）+ 补全字段。
func RegisterInfo(i Info) {
	// 兼容：只给 Type 没给 Capabilities → 先补齐，再参与合并（否则第二次注册覆盖第一次的能力）
	if len(i.Capabilities) == 0 && i.Type != "" {
		i.Capabilities = []string{i.Type}
	}
	old, ok := infos[i.Name]
	if ok {
		// 合并能力（去重）
		seen := map[string]bool{}
		merged := append([]string(nil), old.Capabilities...)
		for _, c := range merged {
			seen[c] = true
		}
		for _, c := range i.Capabilities {
			if !seen[c] {
				merged = append(merged, c)
				seen[c] = true
			}
		}
		i.Capabilities = merged
		// 补全空字段（保留第一次注册的展示信息）
		if i.DisplayName == "" {
			i.DisplayName = old.DisplayName
		}
		if i.Icon == "" {
			i.Icon = old.Icon
		}
		if i.Version == "" {
			i.Version = old.Version
		}
		if i.Desc == "" {
			i.Desc = old.Desc
		}
		if i.Order == 0 {
			i.Order = old.Order
		}
	}
	infos[i.Name] = i
}

// InfoOf 按平台 key 取元数据（type 参数兼容旧调用，忽略）。
func InfoOf(_type, name string) Info { return infos[name] }

// AllInfos 全部插件元数据（每个平台一条）。
func AllInfos() []Info {
	out := make([]Info, 0, len(infos))
	for _, v := range infos {
		out = append(out, v)
	}
	sortInfos(out)
	return out
}

// HasCapability 某平台是否具备某能力。
func HasCapability(name, cap string) bool {
	i, ok := infos[name]
	if !ok {
		return false
	}
	for _, c := range i.Capabilities {
		if c == cap {
			return true
		}
	}
	return false
}

// orderOverride 用户自定义顺序（key=平台 Name → 顺序号）。
var orderOverride = map[string]int{}

func SetOrderOverride(m map[string]int) { orderOverride = m }
func OrderOverride() map[string]int     { return orderOverride }

func sortInfos(list []Info) {
	rank := func(i Info) (int, int) {
		if v, ok := orderOverride[i.Name]; ok {
			return 0, v
		}
		return 1, i.Order
	}
	less := func(a, b Info) bool {
		ag, as := rank(a)
		bg, bs := rank(b)
		if ag != bg {
			return ag < bg
		}
		if as != bs {
			return as < bs
		}
		return a.Name < b.Name // tie-break：map 遍历随机，必须按 Name 定序（否则列表每帧晃）
	}
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && less(list[j], list[j-1]); j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
}

// InfoNames 取具备指定能力的平台元数据（按用户顺序/meta.Order）。
// 兼容旧调用：InfoNames("source") = 所有有 source 能力的平台。
func InfoNames(capability string) []Info {
	var out []Info
	for _, v := range infos {
		if HasCapability(v.Name, capability) {
			out = append(out, v)
		}
	}
	sortInfos(out)
	return out
}

// --- 插件启用/禁用（按平台 key） ---

var enabled = map[string]bool{}

// SetEnabled 按平台 key 设置（type 参数兼容旧调用，忽略）。
func SetEnabled(_type, name string, on bool) { enabled[name] = on }

// IsEnabled 按平台 key 读（type 参数兼容旧调用，忽略）。
func IsEnabled(_type, name string) bool {
	if v, ok := enabled[name]; ok {
		return v
	}
	return true
}
