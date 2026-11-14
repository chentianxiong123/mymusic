//go:build native

package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/egoist/mygo/audio"

	"homecast/service/playback"

	"mymusic-mygo/bilibili"
)

// MyGoOutput 实现主线 playback.Output：MyGo 本机出声后端。
// 区域：只管"怎么真正出声 + 上报进度"（audio.Player），不持有业务状态——
// 状态/队列/模式/持久化全在主线 playback 域包（SSOT）。
//
// 语义对齐主线"web 前端 audio"：MyGo 是本机 audio 出声（等价 HTMLAudio），
// source=web，进度由 MyGo 上报（等价 timeupdate）。与 web 不同的是 MyGo 直接
// 在这里取流+解码出声（不走浏览器），这是"原生前端"的天然接缝。
type MyGoOutput struct {
	bili *bilibili.Client

	mu    sync.Mutex
	p     *audio.Player
	path  string // 当前临时音频文件（Stop/切歌时清理）
	bvid  string
	title string
	gen   int // 切歌代际：切歌后旧 goroutine 的下载放弃（防竞态）
	ended func()
}

func newMyGoOutput(bili *bilibili.Client) *MyGoOutput {
	return &MyGoOutput{bili: bili}
}

// Play 异步：立即返回（UI 不卡），后台取流→下载→解码→出声。
// gen 递增使旧下载任务作废（快速切歌时 A 下载中切 B，A 完成后自动丢弃）。
func (o *MyGoOutput) Play(bvid string, opts playback.PlayOpts) error {
	o.mu.Lock()
	o.gen++
	gen := o.gen
	o.mu.Unlock()
	go o.loadAndPlay(bvid, opts, gen)
	return nil
}

func (o *MyGoOutput) loadAndPlay(bvid string, opts playback.PlayOpts, gen int) {
	// 取流
	info, err := o.bili.GetVideoInfo(bvid)
	if err != nil {
		log.Printf("[mygo-output] 取视频信息失败 %s: %v", bvid, err)
		return
	}
	stream, err := o.bili.GetBestAudioURL(bvid, info.CID, bilibili.PreferredQuality)
	if err != nil {
		log.Printf("[mygo-output] 取流失败 %s: %v", bvid, err)
		return
	}

	// 下载到临时文件（B站 CDN 多节点，拒连重试）
	f, err := os.CreateTemp("", "mymusic-mygo-*.m4a")
	if err != nil {
		log.Printf("[mygo-output] 临时文件失败: %v", err)
		return
	}
	path := f.Name()
	var resp *http.Response
	for attempt := 0; attempt < 5; attempt++ {
		resp, err = o.bili.GetRaw(stream.URL, nil)
		if err == nil {
			break
		}
		time.Sleep(600 * time.Millisecond)
	}
	if err != nil {
		f.Close()
		os.Remove(path)
		log.Printf("[mygo-output] 下载失败 %s: %v", bvid, err)
		return
	}
	_, err = io.Copy(f, resp.Body)
	resp.Body.Close()
	f.Close()
	if err != nil {
		os.Remove(path)
		log.Printf("[mygo-output] 下载失败 %s: %v", bvid, err)
		return
	}

	// 解码
	r, err := os.Open(path)
	if err != nil {
		os.Remove(path)
		log.Printf("[mygo-output] 打开音频失败: %v", err)
		return
	}
	p, err := audio.OpenStream(r)
	if err != nil {
		r.Close()
		os.Remove(path)
		log.Printf("[mygo-output] 解码失败 %s: %v", bvid, err)
		return
	}

	// 提交（若期间已切歌 gen 变了 → 放弃这次，清掉）
	o.mu.Lock()
	if o.gen != gen {
		o.mu.Unlock()
		p.Stop()
		os.Remove(path)
		return
	}
	old := o.p
	oldPath := o.path
	o.p = p
	o.path = path
	o.bvid = bvid
	o.title = opts.Title
	hook := o.ended
	o.mu.Unlock()
	if old != nil {
		old.Stop()
	}
	if oldPath != "" {
		os.Remove(oldPath)
	}
	// 播完回调核心（自动连播：核心填 Next）
	if hook != nil {
		p.OnEnd(func() { hook() })
	}
	p.Play()
}

func (o *MyGoOutput) Pause() error {
	o.mu.Lock()
	p := o.p
	o.mu.Unlock()
	if p != nil {
		p.Pause()
	}
	return nil
}

func (o *MyGoOutput) Resume() error {
	o.mu.Lock()
	p := o.p
	o.mu.Unlock()
	if p != nil {
		p.Play()
	}
	return nil
}

func (o *MyGoOutput) Stop() error {
	o.mu.Lock()
	p := o.p
	path := o.path
	o.p = nil
	o.path = ""
	o.bvid = ""
	o.mu.Unlock()
	if p != nil {
		p.Stop()
	}
	if path != "" {
		os.Remove(path)
	}
	return nil
}

func (o *MyGoOutput) Seek(sec float64) error {
	o.mu.Lock()
	p := o.p
	o.mu.Unlock()
	if p != nil {
		p.Seek(time.Duration(sec * float64(time.Second)))
	}
	return nil
}

func (o *MyGoOutput) SetVolume(v int) error {
	o.mu.Lock()
	p := o.p
	o.mu.Unlock()
	if p != nil {
		p.SetVolume(float64(v) / 100.0)
	}
	return nil
}

// Position 上报当前进度（真实，audio 自报）。BVID/Title 带当前实际在播（对齐核心）。
func (o *MyGoOutput) Position() playback.ProgressInfo {
	o.mu.Lock()
	p := o.p
	bvid := o.bvid
	title := o.title
	o.mu.Unlock()
	if p == nil {
		return playback.ProgressInfo{}
	}
	return playback.ProgressInfo{
		BVID:    bvid,
		Title:   title,
		Pos:     p.Position().Seconds(),
		Dur:     p.Duration().Seconds(),
		Playing: p.Playing(),
	}
}

func (o *MyGoOutput) Caps() playback.Capabilities {
	return playback.Capabilities{Play: true, Pause: true, Resume: true, Stop: true, Seek: true, Volume: true, Progress: true}
}

func (o *MyGoOutput) SetEndedHook(fn func()) {
	o.mu.Lock()
	o.ended = fn
	o.mu.Unlock()
}

func (o *MyGoOutput) Name() string { return "mygo" }

// fmtDurMygo 秒→"m:ss"（UI 用；与主线 fmtDur 对齐）
func fmtDurMygo(sec float64) string {
	if sec <= 0 {
		return "0:00"
	}
	s := int(sec)
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}
