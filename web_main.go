//go:build !native

// web 模式：MyGo 壳 + 原项目 htmx 后端（server.New），主界面就是用户熟悉的 htmx UI。
// 原生 UI 版（native tag）保留在 main.go / player.go / widget.go，可 -tags native 回退。
package main

import (
	"log"

	"github.com/egoist/mygo"

	"homecast/server"
)

func main() {
	// 把原项目的 htmx 服务端渲染 handler 接到 MyGo 的自定义 scheme。
	// 页面源和 API 同源（app://localhost），复用原项目的 htmx + fetch 模型。
	if err := mygo.Protocol.Handle("app", server.New()); err != nil {
		log.Fatal(err)
	}

	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:     "mymusic-mygo",
			Width:     920,
			Height:    720,
			MinWidth:  640,
			MinHeight: 480,
			StateKey:  "main",
			URL:       "app://localhost/",
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
