//go:build native

// restore_config.go —— 启动恢复配置：所有「退出记忆」项统一可配置。
// 一个 JSON 键 mygo_restorecfg 存全部开关；各恢复点读它决定是否恢复。
package main

import (
	"encoding/json"
	"log"

	"github.com/egoist/mygo/ui"

	"homecast/service/conv"
)

// restoreConfig 所有退出记忆开关（默认值 = 现有行为：进度归零/不自动播；
// 其余大多恢复，静音与上次页面默认关）。
type restoreConfig struct {
	Position   bool `json:"position"`   // 恢复播放进度（续播到退出时秒数）
	AutoPlay   bool `json:"autoplay"`   // 自动继续播放（默认关，避免打扰）
	Window     bool `json:"window"`     // 恢复窗口位置/大小/最大化
	Volume     bool `json:"volume"`     // 恢复音量
	Muted      bool `json:"muted"`      // 恢复静音状态（默认关）
	Recent     bool `json:"recent"`     // 恢复最近播放列表
	History    bool `json:"history"`    // 恢复搜索历史
	LastTab    bool `json:"lasttab"`    // 恢复上次页面 tab
	LastSource bool `json:"lastsource"` // 恢复上次搜索音源
	LyricSID   bool `json:"lyricsid"`   // 恢复歌词选源
}

const restoreCfgKey = "mygo_restorecfg"

// defaultRestoreConfig 默认：进度归零/不自动播，其余恢复（静音/页面/歌词选源默认关）。
func defaultRestoreConfig() restoreConfig {
	return restoreConfig{
		Position: false, AutoPlay: false,
		Window: true, Volume: true, Muted: false,
		Recent: true, History: true,
		LastTab: false, LastSource: true, LyricSID: false,
	}
}

// a.restoreCfg 当前生效配置（启动时 load，改时 save）。
// 字段加在 app struct（main.go）。

func (a *app) appLoadRestoreConfig() restoreConfig {
	cfg := defaultRestoreConfig()
	if a.db == nil {
		return cfg
	}
	if v, ok := conv.LoadSetting(a.db, restoreCfgKey); ok && v != "" {
		if json.Unmarshal([]byte(v), &cfg) != nil {
			cfg = defaultRestoreConfig()
		}
	}
	return cfg
}

func (a *app) appSaveRestoreConfig(cfg restoreConfig) {
	a.restoreCfg = cfg
	if a.db == nil {
		return
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return
	}
	if err := conv.SaveSetting(a.db, restoreCfgKey, string(b)); err != nil {
		log.Printf("[restore] 保存配置失败: %v", err)
	}
}

// restoreConfigEntry 设置页只占一行：点击弹窗配置全部启动恢复项。
func (a *app) restoreConfigEntry(c *ui.Context) {
	ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(10 * uiScale).PaddingX(4 * uiScale).Children(func() {
		ui.Text(c, "启动恢复").FontSize(14 * uiScale).Bold().TextColor(clrText)
		ui.Spacer(c).Grow(1)
		if chip(c, "配置 ›").Clicked() {
			a.showRestoreCfg = true
			a.win.Invalidate()
		}
	})
	ui.Text(c, "退出时记住的状态，启动时可分别恢复").FontSize(11 * uiScale).TextColor(clrMuted).PaddingX(4 * uiScale)
}

// restoreConfigDialog 启动恢复配置弹窗（响应式 dialogRect + 滚动，不堆在设置页）。
func (a *app) restoreConfigDialog(c *ui.Context) {
	if !a.showRestoreCfg {
		return
	}
	lx, ly, pw, ph := a.dialogRect(400*uiScale, 460*uiScale)
	ui.Overlay(c, func() {
		bg := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0)
		bg.Draw(func(p *ui.Painter, r ui.Rect) { p.Fill(r, ui.RGBA(0, 0, 0, 0.45), 0) })
		if bg.Clicked() {
			a.showRestoreCfg = false
			a.win.Invalidate()
		}
		panel := ui.Box(c).Absolute().Left(lx).Top(ly).Width(pw).Height(ph).Radius(16 * uiScale).Background(clrCard)
		panel.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Shadow(r, 20, 0, 12, 40, 0, ui.RGBA(0, 0, 0, 0.5))
			p.FillRadii(r, [4]float32{16, 16, 16, 16}, clrCard)
		})
		panel.Children(func() {
			ui.Column(c).Fill().Padding(16 * uiScale).Gap(8 * uiScale).Children(func() {
				ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "启动恢复").FontSize(16 * uiScale).Bold().TextColor(clrText)
					ui.Spacer(c).Grow(1)
					if chip(c, "关闭").Clicked() {
						a.showRestoreCfg = false
						a.win.Invalidate()
					}
				})
				ui.Scroll(c).Grow(1).Scrollbars(false).Children(func() {
					cfg := a.restoreCfg
					items := []struct {
						label, hint string
						value       bool
						set         func(bool)
					}{
						{"恢复播放进度", "续播到上次退出时的播放位置", cfg.Position, func(v bool) { cfg.Position = v }},
						{"自动继续播放", "启动后直接出声（默认关闭）", cfg.AutoPlay, func(v bool) { cfg.AutoPlay = v }},
						{"恢复窗口状态", "位置、大小与最大化状态", cfg.Window, func(v bool) { cfg.Window = v }},
						{"恢复音量", "上次的音量大小", cfg.Volume, func(v bool) { cfg.Volume = v }},
						{"恢复静音状态", "上次是否处于静音（默认关闭）", cfg.Muted, func(v bool) { cfg.Muted = v }},
						{"恢复最近播放", "最近播放列表", cfg.Recent, func(v bool) { cfg.Recent = v }},
						{"恢复搜索历史", "搜索历史记录", cfg.History, func(v bool) { cfg.History = v }},
						{"恢复上次页面", "启动时停留在退出前的页面", cfg.LastTab, func(v bool) { cfg.LastTab = v }},
						{"恢复上次音源", "搜索框选中的音源", cfg.LastSource, func(v bool) { cfg.LastSource = v }},
						{"恢复歌词选源", "歌词页选中的歌词源", cfg.LyricSID, func(v bool) { cfg.LyricSID = v }},
					}
					for _, it := range items {
						it := it
						a.setRowSwitch(c, it.label, it.hint, it.value, func(on bool) {
							it.set(on)
							a.appSaveRestoreConfig(cfg)
							a.win.Invalidate()
						})
					}
				})
			})
		})
	})
}
