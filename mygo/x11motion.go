//go:build native

package main

import (
	"sync"
	"unsafe"

	"github.com/egoist/mygo"
	"github.com/ebitengine/purego"
)

// 全局鼠标移动的事件驱动监听（Linux/X11）：purego 调 Xlib（与 mygo 同方式，
// 不引 cgo），在 root window 上 XSelectInput(PointerMotionMask) + XNextEvent
// 循环收 XMotionEvent。事件驱动、零轮询——只在 X server 投递鼠标移动事件时
// 醒来，没有事件就阻塞在 XNextEvent（不占 CPU）。
//
// 为什么不用 SetInputRegion+Hovered：实测 X11 SHAPE 下穿透窗口 Hovered 不触发。
// 为什么不用轮询：查全局光标只能轮询 CursorScreenPoint；而 Xlib XNextEvent 是
// 真事件驱动（内核/X server 有鼠标移动才投递）。
//
// 局限：现代 X server 若启用 XInput2 且劫持 root motion，传统 PointerMotionMask
// 可能收不到；那样需升级到 XInput2 XI_RawMotion。本实现先用最简 Xlib 事件。

var (
	x11Once      sync.Once
	x11Dpy       uintptr
	x11MotionSub sync.Map // key *int32(lyric 标识) -> 停止标记（简化：单回调）
	x11Callbacks = map[int]func(x, y int32){}
	x11CBSeq     int
	x11CBMu      sync.Mutex

	xOpenDisplay       func(name *byte) uintptr
	xDefaultRootWindow func(dpy uintptr) uintptr
	xSelectInput       func(dpy uintptr, w uintptr, mask uint64) int32
	xNextEvent         func(dpy uintptr, ev *byte) int32
	xFlush             func(dpy uintptr) int32
	xConnectionNumber  func(dpy uintptr) int32
)

const (
	motionNotify = 6 // XEvent.type == MotionNotify
	pointerMotionMask = 1 << 2
)

// startX11Motion 启动一次 X11 全局鼠标移动事件循环（进程级单例）。回调在
// XNextEvent goroutine 里被调用（非主线程）；调用方需保证回调线程安全
// （mygo Window.SetIgnoreMouseEvents/Invalidate 内部投递主线程）。
func startX11Motion() {
	x11Once.Do(func() {
		go x11MotionLoop()
	})
}

// addX11MotionCB 注册一个鼠标移动回调，返回取消函数。
func addX11MotionCB(cb func(x, y int32)) (cancel func()) {
	x11CBMu.Lock()
	id := x11CBSeq
	x11CBSeq++
	x11Callbacks[id] = cb
	x11CBMu.Unlock()
	startX11Motion()
	return func() {
		x11CBMu.Lock()
		delete(x11Callbacks, id)
		x11CBMu.Unlock()
	}
}

func x11MotionLoop() {
	lib, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return
	}
	purego.RegisterLibFunc(&xOpenDisplay, lib, "XOpenDisplay")
	purego.RegisterLibFunc(&xDefaultRootWindow, lib, "XDefaultRootWindow")
	purego.RegisterLibFunc(&xSelectInput, lib, "XSelectInput")
	purego.RegisterLibFunc(&xNextEvent, lib, "XNextEvent")
	purego.RegisterLibFunc(&xFlush, lib, "XFlush")
	purego.RegisterLibFunc(&xConnectionNumber, lib, "XConnectionNumber")

	dpy := xOpenDisplay(nil)
	if dpy == 0 {
		return
	}
	root := xDefaultRootWindow(dpy)
	xSelectInput(dpy, root, pointerMotionMask)
	xFlush(dpy)

	var ev [192]byte // XEvent union（64 位 192 字节）
	for {
		xNextEvent(dpy, &ev[0])
		typ := *(*int32)(unsafe.Pointer(&ev[0]))
		if typ != motionNotify {
			continue
		}
		// XMotionEvent: x_root at 72, y_root at 76（屏幕绝对坐标，与 XKeyEvent state@80 对齐）
		x := *(*int32)(unsafe.Pointer(&ev[72]))
		y := *(*int32)(unsafe.Pointer(&ev[76]))
		x11CBMu.Lock()
		cbs := make([]func(x, y int32), 0, len(x11Callbacks))
		for _, cb := range x11Callbacks {
			cbs = append(cbs, cb)
		}
		x11CBMu.Unlock()
		for _, cb := range cbs {
			cb(x, y)
		}
	}
}

// 确保 mygo 被引用（本文件只用 purego，但保持包内一致）
var _ = mygo.Point{}
