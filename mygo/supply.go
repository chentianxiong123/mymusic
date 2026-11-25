//go:build native

// supply.go —— 统一取流供给（1.2.5 核心抽象）。
// 本机播放 / HTTP 供给端点 / DLNA / 音箱共用这一个入口：
// SongRef → 音源插件 GetStream → m4s 共享缓存 → mp3 转码缓存 → 可播路径。
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"homecast/bilibili"
	"homecast/model"
	"homecast/service/cache"
	"homecast/service/transcode"
	"mymusic-mygo/source"
)

// Supplier 统一取流：ref → 本地可播 mp3/文件路径。
// 音源表按函数注入（避免 mygo→主线的依赖方向问题）。
type Supplier struct {
	sources func() map[string]source.MusicSource
	cache   *cache.CacheService
	xcoder  *transcode.Transcoder
	client  *bilibili.Client // 用于 GetRaw 带 referer（B站多节点重试）
}

// Resolve 把跨源引用解析为可播文件路径。
// 返回 (path, isDirect)：isDirect=true 表示 file:// 直链（本地源，无需缓存/转码）。
func (s *Supplier) Resolve(ref model.SongRef, quality int, qn int) (string, bool, error) {
	if !ref.Valid() {
		return "", false, fmt.Errorf("无效歌曲引用：%s/%s", ref.Source, ref.ID)
	}
	srcs := s.sources()
	src := srcs[ref.Source]
	if src == nil {
		return "", false, fmt.Errorf("音源 %q 未加载", ref.Source)
	}
	st, err := src.GetStream(ref, quality)
	if err != nil {
		return "", false, err
	}
	if st == nil || st.URL == "" {
		return "", false, fmt.Errorf("音源 %q 返回空流", ref.Source)
	}
	// 本地文件：直接播，不缓存不转码
	if strings.HasPrefix(st.URL, "file://") {
		p := strings.TrimPrefix(st.URL, "file://")
		if _, err := os.Stat(p); err != nil {
			return "", false, err
		}
		return p, true, nil
	}
	// mp3 缓存命中（同曲二次秒开；与本机播放/音箱/供给端点共用）
	if s.xcoder != nil {
		if _, ok := s.xcoder.Get(ref.ID, qn); ok {
			p := s.xcoder.FilePath(ref.ID, qn)
			if fi, err := os.Stat(p); err == nil && fi.Size() > 0 {
				return p, false, nil
			}
		}
	}
	// 源 m4s 落共享缓存（唯一源缓存，不重复 IO）
	m4sPath, err := s.ensureSource(ref.ID, qn, st.URL, st.Referer)
	if err != nil {
		return "", false, err
	}
	// 转码 mp3（ffmpeg 缺失降级直放 m4s）
	if s.xcoder == nil || !s.xcoder.Available() {
		return m4sPath, false, nil
	}
	mp3Path, err := s.xcoder.EnsureMP3(m4sPath, ref.ID, qn)
	if err != nil {
		log.Printf("[supply] 转码失败 %s（降级直放源）: %v", ref.ID, err)
		return m4sPath, false, nil
	}
	return mp3Path, false, nil
}

// ensureSource 确保源音频落主线 cache（共用同一份），带 referer + 拒连重试 + 完整性校验。
func (s *Supplier) ensureSource(id string, qn int, url, referer string) (string, error) {
	if s.cache != nil {
		if _, _, ok := s.cache.Get(id); ok {
			p := s.cache.FilePath(id)
			if fi, err := os.Stat(p); err == nil && fi.Size() > 0 {
				return p, nil
			}
		}
	}
	if url == "" {
		return "", fmt.Errorf("empty audio url")
	}
	var resp *http.Response
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		hdr := http.Header{}
		if referer != "" {
			hdr.Set("Referer", referer)
		}
		if s.client != nil {
			resp, err = s.client.GetRaw(url, hdr)
		} else {
			req, _ := http.NewRequest(http.MethodGet, url, nil)
			for k, vs := range hdr {
				for _, v := range vs {
					req.Header.Add(k, v)
				}
			}
			resp, err = http.DefaultClient.Do(req)
		}
		if err == nil {
			break
		}
		time.Sleep(600 * time.Millisecond)
	}
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.ContentLength > 0 && int64(len(data)) != resp.ContentLength {
		return "", fmt.Errorf("truncated %d/%d", len(data), resp.ContentLength)
	}
	if s.cache != nil {
		if err := s.cache.Put(id, qn, data); err != nil {
			log.Printf("[supply] 回写缓存失败 %s: %v（不影响播放）", id, err)
		}
		return s.cache.FilePath(id), nil
	}
	f, err := os.CreateTemp("", "mymusic-supply-*.src")
	if err != nil {
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return f.Name(), nil
}
