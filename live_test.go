//go:build live

package main

import (
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/audio"
	_ "github.com/egoist/mygo/audio/aac"
	_ "github.com/egoist/mygo/audio/mp4"
	"github.com/egoist/mygo/ui"

	"mymusic-mygo/bilibili"
)

// TestLivePlay 验证取流→下载→fMP4 解码闭环（临时验证用，可删）。
func TestLivePlay(t *testing.T) {
	c := bilibili.NewClient(
		"https://api.bilibili.com",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"https://search.bilibili.com/",
		30*time.Second,
	)
	r, err := c.Search("周杰伦", 1, 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	item := r.Result[0]
	t.Logf("pick %s %s", item.BVID, item.Title)

	info, err := c.GetVideoInfo(item.BVID)
	if err != nil {
		t.Fatalf("videoinfo: %v", err)
	}
	t.Logf("cid=%d title=%s", info.CID, info.Title)

	stream, err := c.GetBestAudioURL(item.BVID, info.CID, bilibili.PreferredQuality)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	t.Logf("url=%s mime=%s codecs=%s bw=%d size=%d", stream.URL, stream.MimeType, stream.Codecs, stream.Bandwidth, stream.Size)

	resp, err := c.GetRaw(stream.URL, nil)
	if err != nil {
		t.Fatalf("getraw: %v", err)
	}
	f, _ := os.CreateTemp("", "mymusic-*.m4a")
	defer os.Remove(f.Name())
	n, err := io.Copy(f, resp.Body)
	resp.Body.Close()
	f.Close()
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	t.Logf("downloaded %d bytes", n)

	rf, _ := os.Open(f.Name())
	defer rf.Close()
	p, err := audio.OpenStream(rf)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	t.Logf("decoded OK, duration=%s", p.Duration())
	if p.Duration() <= 0 {
		t.Fatalf("duration 0, fMP4 未解析出时长")
	}
}

// TestCover 验证封面图下载+解码（临时，可删）。
func TestCover(t *testing.T) {
	c := bilibili.NewClient(
		"https://api.bilibili.com",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"https://search.bilibili.com/", 30*time.Second,
	)
	r, err := c.Search("周杰伦", 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	pic := r.Result[0].Pic
	t.Logf("pic=%s", pic)
	if len(pic) > 2 && pic[:2] == "//" {
		pic = "https:" + pic
	} else if len(pic) > 7 && pic[:7] == "http://" {
		pic = "https://" + pic[7:]
	}
	req, _ := http.NewRequest("GET", pic, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0")
	req.Header.Set("Referer", "https://www.bilibili.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	t.Logf("downloaded %d bytes code=%d", len(data), resp.StatusCode)
	img, err := ui.DecodeImage(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	t.Logf("decode OK %v", img)
}
