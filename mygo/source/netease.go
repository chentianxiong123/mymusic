// Package source: 网易云音源（Go 内置）。明文 API：搜索 /api/search/get/web、
// 取流 /api/song/enhance/player/url。Cookie 可选（扫码登录后 VIP/无损）。
// 歌词复用主线 lyric 域（跟 JS 版一致，不在这里实现）。
package source

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"homecast/service/lyric"
	"mymusic-mygo/model"
)

type Netease struct{ cookie string }

func NewNetease(cookie string) *Netease { return &Netease{cookie: cookie} }

var _ MusicSource = (*Netease)(nil)

func (n *Netease) Name() string { return "netease" }

func (n *Netease) headers() http.Header {
	h := http.Header{}
	h.Set("Referer", "https://music.163.com/")
	h.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0 Safari/537.36")
	c := "NMTID=; appver=8.7.01; os=pc; channel=netease"
	if n.cookie != "" {
		c += "; " + n.cookie
	}
	h.Set("Cookie", c)
	return h
}

func neteaseGet(u string, h http.Header) ([]byte, error) {
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header = h
	cl := &http.Client{Timeout: 15 * time.Second}
	resp, err := cl.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (n *Netease) Search(keyword string, page, size int) ([]Song, int, error) {
	if page < 1 {
		page = 1
	}
	if size <= 0 {
		size = 30
	}
	u := fmt.Sprintf("https://music.163.com/api/search/get/web?s=%s&type=1&offset=%d&limit=%d&total=true",
		url.QueryEscape(keyword), (page-1)*size, size)
	body, err := neteaseGet(u, n.headers())
	if err != nil {
		return nil, 0, err
	}
	var resp struct {
		Result struct {
			Songs []struct {
				ID       int64  `json:"id"`
				Name     string `json:"name"`
				Artists  []struct{ Name string `json:"name"` } `json:"artists"`
				Album    struct {
					Name   string `json:"name"`
					PicURL string `json:"picUrl"`
				} `json:"album"`
				Duration int64 `json:"duration"`
				Fee      int   `json:"fee"`
			} `json:"songs"`
			SongCount int `json:"songCount"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, 0, err
	}
	out := make([]Song, 0, len(resp.Result.Songs))
	for _, s := range resp.Result.Songs {
		artists := make([]string, 0, len(s.Artists))
		for _, a := range s.Artists {
			artists = append(artists, a.Name)
		}
		cover := s.Album.PicURL
		if cover == "" && len(s.Artists) > 0 {
			cover = "https://music.163.com/api/img1v1/2/" + strconv.FormatInt(s.ID, 10)
		}
		sec := int(s.Duration / 1000)
		out = append(out, Song{
			ID:          strconv.FormatInt(s.ID, 10),
			Title:       s.Name,
			Artist:      strings.Join(artists, "/"),
			Cover:       cover,
			Duration:    formatDur(sec),
			DurationSec: sec,
			Typename:    "netease",
			Album:       s.Album.Name,
			VIP:         s.Fee > 0,
		})
	}
	return out, resp.Result.SongCount, nil
}

func (n *Netease) GetStream(id string, quality int) (*Stream, error) {
	br := 320000
	if quality == 0 {
		br = 128000
	}
	u := fmt.Sprintf("https://music.163.com/api/song/enhance/player/url?ids=[%s]&br=%d", url.PathEscape(id), br)
	body, err := neteaseGet(u, n.headers())
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data []struct {
			URL  string  `json:"url"`
			Time float64 `json:"time"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	if len(resp.Data) == 0 || resp.Data[0].URL == "" {
		return nil, fmt.Errorf("无权限或需登录（VIP 歌免登录不可播）")
	}
	return &Stream{URL: resp.Data[0].URL, Quality: quality, Duration: resp.Data[0].Time / 1000}, nil
}

func (n *Netease) GetLyric(id string, keywords []string, sid int) ([]model.LyricLine, error) {
	res := lyric.GetLyricLines(keywords, sid, 0)
	if res == nil {
		return nil, nil
	}
	out := make([]model.LyricLine, len(res.Lines))
	for i, ln := range res.Lines {
		sec, _ := ln[0].(float64)
		text, _ := ln[1].(string)
		out[i] = model.LyricLine{Time: sec, Text: text}
	}
	return out, nil
}

func formatDur(sec int) string {
	return fmt.Sprintf("%d:%02d", sec/60, sec%60)
}
