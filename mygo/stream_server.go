//go:build native

// 音箱供给端点 + 歌词端点：MyGo 起 HTTP server（对齐主线 server 路由 /api/v1/*）。
//  1. 音箱播放 URL（speaker.unifiedAudioURL）→ /api/v1/music/stream/{bvid}：
//     内部走主线 api.MusicHandler（音箱 did 非空秒开分支 = transcode 转 mp3 + seek 续播）。
//  2. 歌词来源（候选搜索/取行/选源/偏移）→ 主线 api.LyricHandler（与主线 web 端同源）：
//     mygo 歌词页/挂件从本地 HTTP 拉取，来源逻辑 100% 复用主线（LyricKeywords 分P标题 +
//     LyricCache 透明缓存 + GetLyricLines 自动最优 + Select/Offset 持久化），不自创候选拼装。
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"

	"homecast/api"
	"homecast/service/lyric"
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
	// 歌词来源：主线 LyricHandler（LyricStore 选源持久化 + Music.LyricKeywords 分P标题
	// + LyricCache 透明缓存 + GetLyricLines 自动最优——与主线 web 端 /music/lyric 同源）。
	lh := &api.LyricHandler{
		Lyric:      a.svc.lyricStore,
		Music:      a.svc.music,
		LyricCache: a.svc.lyricCache,
	}
	mux := http.NewServeMux()
	// 对齐主线 router.go 的歌词路由（五端点全部挂上）
	mux.HandleFunc("GET /api/v1/music/lyric", lh.Get)
	mux.HandleFunc("POST /api/v1/lyric/select", lh.Select)
	mux.HandleFunc("GET /api/v1/music/lyric/candidates", lh.Candidates)
	mux.HandleFunc("GET /api/v1/lyric/meta", lh.Meta)
	mux.HandleFunc("POST /api/v1/lyric/offset", lh.Offset)
	mux.HandleFunc("GET /api/v1/music/stream/{bvid}", mh.Stream)
	addr := ":" + port
	log.Printf("[stream] 供给端点 http://%s/api/v1/music/stream/{bvid} + /music/lyric*", addr)
	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Printf("[stream] 供给端点退出: %v", err)
		}
	}()
}

func (a *app) lyricBaseURL() string {
	port := os.Getenv("HC_PORT")
	if port == "" {
		port = "28977"
	}
	return "http://127.0.0.1:" + port
}

// apiEnv {code,data} 信封（对齐主线 api.ok/writeJSON）。
// lyricGetBVID bvid 模式取歌词行：与主线 web 端 loadBackendLyric 同源。
// 服务端 keywordsFor（分P标题优先）→ LyricCache 透明缓存 → SelectedSID/
// GetLyricLines 自动最优；返回选中源行列表+偏移；data=null=未找到歌词。
func (a *app) lyricGetBVID(bvid, keyword string) (*lyric.LyricResult, error) {
	u := a.lyricBaseURL() + "/api/v1/music/lyric?bvid=" + bvid + "&keyword=" + keyword
	env, err := a.lyricJSON(u)
	if err != nil {
		return nil, err
	}
	if env == nil || len(*env) == 0 || string(*env) == "null" {
		return nil, nil // code!=0 或 data=null（未找到歌词，正常业务）
	}
	var res lyric.LyricResult
	if err := json.Unmarshal(*env, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// lyricCandsViaHTTP 候选元数据列表（与主线 web 端 lycSelLazy 前段同源，全部候选均无正文，
// 选中后由后端 /music/lyric?sid= 懒拉）。
func (a *app) lyricCandsViaHTTP(keyword, bvid string, limit int) ([]lyric.LyricResult, error) {
	u := fmt.Sprintf("%s/api/v1/music/lyric/candidates?keyword=%s&bvid=%s&limit=%d",
		a.lyricBaseURL(), keyword, bvid, limit)
	env, err := a.lyricJSON(u)
	if err != nil {
		return nil, err
	}
	if env == nil || len(*env) == 0 || string(*env) == "null" {
		return nil, nil
	}
	var out []lyric.LyricResult
	if err := json.Unmarshal(*env, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// lyricSelect 选源上报（POST /api/v1/lyric/select，SQLite 持久化 + 清歌词缓存 + 广播）
func (a *app) lyricSelect(bvid string, c *lyric.LyricResult) error {
	if c == nil {
		return nil
	}
	u := fmt.Sprintf("%s/api/v1/lyric/select?bvid=%s&sid=%d&name=%s&artist=%s&duration=%d",
		a.lyricBaseURL(), bvid, c.ID, urlQE(c.Name), urlQE(c.Artist), c.Duration)
	_, err := a.lyricPost(u)
	return err
}

// lyricSetOffset 对齐偏移持久化（POST /api/v1/lyric/offset，SQLite + 清缓存 + 广播）
func (a *app) lyricSetOffsetHTTP(bvid string, off float64) error {
	u := fmt.Sprintf("%s/api/v1/lyric/offset?bvid=%s&offset=%f", a.lyricBaseURL(), bvid, off)
	_, err := a.lyricPost(u)
	return err
}

// lyricPost POST 并解析信封（code==0 或无 body 都当成功返回）。
func (a *app) lyricPost(u string) (*json.RawMessage, error) {
	resp, err := http.Post(u, "application/json", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var env struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, err
	}
	if env.Code != 0 {
		return nil, fmt.Errorf("lyric api code=%d", env.Code)
	}
	return &env.Data, nil
}

// lyricJSON GET 并解析信封（对齐 api.ok：code==0 才有效；code!=0 按 nil 返回）。
func (a *app) lyricJSON(u string) (*json.RawMessage, error) {
	resp, err := http.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var env struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, err
	}
	if env.Code != 0 {
		return nil, nil
	}
	return &env.Data, nil
}

// urlQE url.QueryEscape 短别名（歌词名称/歌手可能含中文与空格）。
func urlQE(s string) string {
	esc := url.QueryEscape(s)
	return esc
}
