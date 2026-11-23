//go:build native

// window_state.go —— 桌面窗口状态持久化（位置/大小/最大化）。
//
// 存 settings 表一个 JSON 键 mygo_windowstate：{"x":..,"y":..,"w":..,"h":..,"max":..}
// 时机：
//   - OnMove / OnResize：节流 1s（拖动/拉伸时高频，不每次都写盘）
//   - OnClose：退出时无条件立即写（保证最后状态不丢）
// 读：创建窗口前 restoreWindowState()；无记录（或明显非法）则用默认 980×680 居中。
package main

import (
	"encoding/json"
	"log"
	"time"

	"github.com/egoist/mygo"

	"homecast/service/conv"
)

const windowStateKey = "mygo_windowstate"

// winState 窗口状态（JSON 存 settings）。
type winState struct {
	X   int  `json:"x"`
	Y   int  `json:"y"`
	W   int  `json:"w"`
	H   int  `json:"h"`
	Max bool `json:"max"`
}

// 保存节流间隔：拖动/拉伸每帧都触发 OnMove/OnResize，1s 一次足够。
const winStateSaveInterval = time.Second

var lastWinStateSave time.Time

// restoreWindowState 读上次的窗口状态；无记录/损坏/非法返回 nil（用默认）。
// 合法性：w/h 至少 400×300（防止记录到最小化/异常值导致下次窗口打不开）。
func (a *app) restoreWindowState() *winState {
	if a.db == nil {
		return nil
	}
	v, ok := conv.LoadSetting(a.db, windowStateKey)
	if !ok || v == "" {
		return nil
	}
	var s winState
	if json.Unmarshal([]byte(v), &s) != nil {
		return nil
	}
	if s.W < 400 || s.H < 300 {
		return nil
	}
	return &s
}

// saveWindowState 立即写当前窗口状态到 settings（退出/节流定时器调用）。
func (a *app) saveWindowState(win *mygo.Window) {
	if a.db == nil || win == nil {
		return
	}
	b := win.Bounds()
	s := winState{X: b.X, Y: b.Y, W: b.Width, H: b.Height, Max: win.IsMaximized()}
	data, err := json.Marshal(s)
	if err != nil {
		return
	}
	if err := conv.SaveSetting(a.db, windowStateKey, string(data)); err != nil {
		log.Printf("[winstate] 保存失败：%v", err)
		return
	}
	lastWinStateSave = time.Now()
}

// saveWindowStateThrottled 高频事件（OnMove/OnResize）用：1s 内只写一次。
// 最后一次状态由 OnClose 保证落盘。
func (a *app) saveWindowStateThrottled(win *mygo.Window) {
	if time.Since(lastWinStateSave) < winStateSaveInterval {
		return
	}
	a.saveWindowState(win)
}
