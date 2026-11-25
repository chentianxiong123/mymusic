//go:build native

// stream_ref.go —— 1.2.5 通用供给端点 handler。
// GET /api/v1/music/stream/ref?source=xxx&id=yyy&quality=0..2
// 按 SongRef 走统一 Supplier（音源插件 → m4s 缓存 → mp3 转码），
// 本地源直接 serve 文件。DLNA/外部播放器/蓝牙都可用。
package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"homecast/bilibili"
	"homecast/model"
)

func (a *app) streamRefHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ref := model.SongRef{
		Source: q.Get("source"),
		ID:     q.Get("id"),
		Title:  q.Get("title"),
	}
	if ref.Source == "" || ref.ID == "" {
		http.Error(w, "source and id required", http.StatusBadRequest)
		return
	}
	quality, _ := strconv.Atoi(q.Get("quality"))
	qn := bilibili.PreferredQuality
	switch quality {
	case 0:
		qn = bilibili.Audio64K
	case 1:
		qn = bilibili.Audio128K
	}
	if a.supplier == nil {
		http.Error(w, "supplier not ready", http.StatusServiceUnavailable)
		return
	}
	path, direct, err := a.supplier.Resolve(ref, quality, qn)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if direct {
		w.Header().Set("Content-Type", contentTypeOf(path))
	} else {
		w.Header().Set("Content-Type", "audio/mpeg")
	}
	// Range 支持（DLNA/播放器拖动进度）
	f, err := os.Open(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer f.Close()
	st, _ := f.Stat()
	http.ServeContent(w, r, filepath.Base(path), st.ModTime(), f)
}

func contentTypeOf(path string) string {
	switch filepath.Ext(strings.ToLower(path)) {
	case ".mp3":
		return "audio/mpeg"
	case ".flac":
		return "audio/flac"
	case ".m4a", ".mp4":
		return "audio/mp4"
	case ".ogg":
		return "audio/ogg"
	case ".wav":
		return "audio/wav"
	}
	return fmt.Sprintf("application/octet-stream")
}
