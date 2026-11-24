//go:build native

// plugin_order.go —— 插件顺序的用户自定义（持久化 settings 表）。
//
// 顺序影响所有表征：插件页列表、搜索下拉框（SourceNames）、目标列表等。
// 存 settings 键 mygo_pluginorder，JSON: {"source/netease":0,"source/bilibili":1,...}
// 值 = 排序号（小在前）。未记录的插件保持 meta.Order 默认顺序。
package main

import (
	"encoding/json"
	"log"

	"mymusic-mygo/plugin"

	"homecast/service/conv"
)

const pluginOrderKey = "mygo_pluginorder"

// restorePluginOrder 启动恢复用户顺序（在 SourceNames 首次调用前执行）。
func (a *app) restorePluginOrder() {
	if a.db == nil {
		return
	}
	v, ok := conv.LoadSetting(a.db, pluginOrderKey)
	if !ok || v == "" {
		return
	}
	var m map[string]int
	if json.Unmarshal([]byte(v), &m) != nil {
		return
	}
	plugin.SetOrderOverride(m)
}

// restorePluginEnabled 启动恢复插件启用状态：遍历所有已注册插件，读 settings 键
// plugin_<type>_<name>（0=关，其它/缺省=开）。歌词源开关是核心配置，重启必须生效。
func (a *app) restorePluginEnabled() {
	if a.db == nil {
		return
	}
	for _, in := range plugin.AllInfos() {
		key := "plugin_" + in.Type + "_" + in.Name
		v, ok := conv.LoadSetting(a.db, key)
		if ok && v == "0" {
			plugin.SetEnabled(in.Type, in.Name, false)
		}
	}
}

// savePluginOrder 把当前顺序覆盖表写回 settings（用户调整顺序后调）。
func (a *app) savePluginOrder() {
	if a.db == nil {
		return
	}
	data, err := json.Marshal(plugin.OrderOverride())
	if err != nil {
		return
	}
	if err := conv.SaveSetting(a.db, pluginOrderKey, string(data)); err != nil {
		log.Printf("[pluginOrder] 保存失败：%v", err)
	}
}

// movePlugin 把 kind 类型的第 idx 个插件上移/下移（delta=-1/+1），重排+持久化。
// 做法：取当前排序列表 → 交换相邻两项 → 把全量新顺序写进 override（含未移动的插件，
// 否则只写被移动的俩，其它插件顺序会跳回 meta.Order）。
func (a *app) movePlugin(kind string, idx, delta int) {
	list := plugin.InfoNames(kind)
	j := idx + delta
	if idx < 0 || j < 0 || j >= len(list) {
		return
	}
	list[idx], list[j] = list[j], list[idx]
	// 全量写回：每个插件的新顺序号 = 列表索引（跨类型也保留各自原 override/Order）
	m := map[string]int{}
	for i, in := range list {
		m[in.Type+"/"+in.Name] = i
	}
	// 保留其它类型的已有 override（不丢其它分组的顺序）
	for k, v := range plugin.OrderOverride() {
		if len(k) > 0 && k[:min(len(k), len(kind)+1)] != kind+"/" {
			m[k] = v
		}
	}
	plugin.SetOrderOverride(m)
	a.savePluginOrder()
	a.win.Invalidate()
}

// reorderPlugins 官方 ui.List 的 Reorder 回调：rows=被拖行索引，to=插入位置（原列表第 to 行之前）。
// 重排该类型插件 → 写 orderOverride + 持久化（影响插件页/搜索下拉框等所有表征）。
func (a *app) reorderPlugins(kind string, rows []int, to int) {
	list := plugin.InfoNames(kind)
	if len(list) == 0 || len(rows) == 0 {
		return
	}
	chosen := map[int]bool{}
	for _, r := range rows {
		chosen[r] = true
	}
	moved, rest := []plugin.Info{}, []plugin.Info{}
	at := 0
	for i, in := range list {
		if i == to {
			at = len(rest)
		}
		if chosen[i] {
			moved = append(moved, in)
		} else {
			rest = append(rest, in)
		}
	}
	if to >= len(list) {
		at = len(rest)
	}
	ordered := make([]plugin.Info, 0, len(list))
	ordered = append(ordered, rest[:at]...)
	ordered = append(ordered, moved...)
	ordered = append(ordered, rest[at:]...)
	m := map[string]int{}
	for _, in := range ordered {
		m[in.Type+"/"+in.Name] = len(m)
	}
	for k, v := range plugin.OrderOverride() {
		if len(k) <= len(kind) || k[:len(kind)+1] != kind+"/" {
			m[k] = v // 保留其它类型
		}
	}
	plugin.SetOrderOverride(m)
	a.savePluginOrder()
	a.win.Invalidate()
}
