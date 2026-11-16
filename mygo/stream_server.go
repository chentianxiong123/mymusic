//go:build native

// 音箱供给端点：MyGo 起 HTTP server 供音箱拉流（对齐主线 server /api/v1/music/stream/{bvid}）。
// 音箱播放 URL（speaker.unifiedAudioURL）指向 http://<ip>:<port>/api/v1/music/stream/{bvid}，
// 该端点内部走主线 api.MusicHandler：音箱（did 非空）秒开分支 = transcode 转 mp3 + seek 续播；
// 无 ffmpeg / 未缓存时降级原 m4s 路径（与主线同逻辑，SSOT 完全复用）。
package main

import (
	"log"
	"net/http"
	"os"

	"homecast/api"
)

// startStreamServer 起音箱供给端点。端口独立于主线（默认 28977，可 HC_PORT 覆盖）：
// 避免与主线后端（28976）冲突；音箱播放 URL 的端口由 speaker 服务读 HC_PORT 拼出，
// 因此这里设 os.Setenv("HC_PORT", ...) 让音箱拉本供给端点，而不是主线。
func (a *app) startStreamServer() {
	port := os.Getenv("HC_PORT")
	if port == "" {
		port = "28977"
	}
	os.Setenv("HC_PORT", port)
	if a.svc == nil || a.svc.music == nil || a.svc.xcoder == nil || a.svc.cache == nil {
		log.Printf("[stream] 供给端点跳过：域包未就绪")
		return
	}
	// 复用主线 api.MusicHandler（公开后 mygo 可直接 import）：Stream 端点含音箱秒开分支。
	// music=取流（GetAudioStream）；cache=音频缓存（命中秒开 + 转码源 m4s）；
	// xcoder=软转码 mp3；gate=音箱断源闸门（暂停/停止 → 拒绝其拉流，对齐主线）。
	mh := api.NewMusicHandler(a.svc.music)
	mh.Cache = a.svc.cache
	mh.Xcoder = a.svc.xcoder
	mh.Gate = a.svc.speaker
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/music/stream/{bvid}", mh.Stream)
	addr := ":" + port
	log.Printf("[stream] 音箱供给端点 http://%s/api/v1/music/stream/{bvid}", addr)
	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Printf("[stream] 供给端点退出: %v", err)
		}
	}()
}