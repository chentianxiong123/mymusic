//go:build native

// chart.go —— 公共榜单面板（独立「排行」板块，无登录、跨源）。
// 榜单 ≠ 推荐：这里是平台公开热榜，与推荐/分类并列，不混入推荐插件体系。
// 实测（2026-11-25）：
//   网易云 /api/toplist 列榜单 → /api/playlist/detail?id= 拿 100 首歌（无登录）
//   酷狗 /api/v3/rank/list 列榜单 → /rank/song?rankid=8888&version=9108 拿歌（无登录）

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
)

// ChartSong 榜单一首歌（跨源通用；source+id 真实，不塞进 BVID 撒谎）。
type ChartSong struct {
	Source string
	ID     string
	Title  string
	Artist string
	Cover  string
}

// chartList 一个榜单（平台名 + 榜名 + 歌曲）。
type chartList struct {
	Platform string
	Name     string
	Songs    []ChartSong
}

// chartFetcher 榜单数据抓取（各平台实现）。
type chartFetcher interface {
	name() string        // 平台展示名
	fetch() ([]chartList, error)
}

var chartHTTP = &http.Client{Timeout: 12 * time.Second}

func chartGet(u, referer string) ([]byte, error) {
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("Referer", referer)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0 Safari/537.36")
	resp, err := chartHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// ---- 网易云榜单 ----
type neteaseChart struct{}

func (neteaseChart) name() string { return "网易云" }

func (neteaseChart) fetch() ([]chartList, error) {
	body, err := chartGet("https://music.163.com/api/toplist", "https://music.163.com/")
	if err != nil {
		return nil, err
	}
	var top struct {
		Code int `json:"code"`
		List []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"list"`
	}
	if err := json.Unmarshal(body, &top); err != nil {
		return nil, err
	}
	// 取前 4 个榜（避免 8 个全拉爆；用户点开详情可再扩）
	out := make([]chartList, 0, 4)
	for _, t := range top.List {
		if len(out) >= 4 {
			break
		}
		if t.ID == 0 || t.Name == "" {
			continue
		}
		songs, err := neteaseChartSongs(t.ID)
		if err != nil || len(songs) == 0 {
			continue
		}
		out = append(out, chartList{Platform: "网易云", Name: t.Name, Songs: songs})
	}
	return out, nil
}

func neteaseChartSongs(id int64) ([]ChartSong, error) {
	u := fmt.Sprintf("https://music.163.com/api/playlist/detail?id=%d", id)
	body, err := chartGet(u, "https://music.163.com/")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Code   int `json:"code"`
		Result struct {
			Tracks []struct {
				ID     int64 `json:"id"`
				Name   string `json:"name"`
				Artists []struct{ Name string `json:"name"` } `json:"artists"`
				Album  struct{ PicURL string `json:"picUrl"` } `json:"album"`
			} `json:"tracks"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	out := make([]ChartSong, 0, len(resp.Result.Tracks))
	for _, t := range resp.Result.Tracks {
		artist := ""
		if len(t.Artists) > 0 {
			artist = t.Artists[0].Name
		}
		out = append(out, ChartSong{
			Source: "netease", ID: strconv.FormatInt(t.ID, 10),
			Title: t.Name, Artist: artist, Cover: t.Album.PicURL,
		})
		if len(out) >= 20 {
			break
		}
	}
	return out, nil
}

// ---- 酷狗榜单 ----
type kugouChart struct{}

func (kugouChart) name() string { return "酷狗" }

func (kugouChart) fetch() ([]chartList, error) {
	body, err := chartGet("http://mobilecdn.kugou.com/api/v3/rank/list?plat=0&version=9108&showtype=2&apiver=2", "http://mobile.kugou.com/")
	if err != nil {
		return nil, err
	}
	var top struct {
		Data struct {
			Info []struct {
				RankID   int    `json:"rankid"`
				RankName string `json:"rankname"`
			} `json:"info"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &top); err != nil {
		return nil, err
	}
	out := make([]chartList, 0, 4)
	// TOP500(8888) 优先，其余取前 3
	ids := []int{8888}
	for _, t := range top.Data.Info {
		if len(ids) >= 4 {
			break
		}
		if t.RankID != 8888 && t.RankID != 0 {
			ids = append(ids, t.RankID)
		}
	}
	nameOf := map[int]string{8888: "TOP500"}
	for _, t := range top.Data.Info {
		nameOf[t.RankID] = t.RankName
	}
	for _, id := range ids {
		songs := kugouChartSongs(id)
		if len(songs) == 0 {
			continue
		}
		out = append(out, chartList{Platform: "酷狗", Name: nameOf[id], Songs: songs})
	}
	return out, nil
}

func kugouChartSongs(rankID int) []ChartSong {
	u := fmt.Sprintf("http://mobilecdn.kugou.com/api/v3/rank/song?rankid=%d&plat=0&page=1&pagesize=20&version=9108&showtype=2&apiver=2&with_res_tag=0", rankID)
	body, err := chartGet(u, "http://mobile.kugou.com/")
	if err != nil {
		return nil
	}
	var resp struct {
		Errcode int `json:"errcode"`
		Data    struct {
			Info []struct {
				SongName string `json:"songname"`
				SingerName string `json:"singername"`
				AlbumID  string `json:"album_id"`
				Hash     string `json:"hash"`
			} `json:"info"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.Errcode != 0 {
		return nil
	}
	out := make([]ChartSong, 0, len(resp.Data.Info))
	for _, s := range resp.Data.Info {
		cover := ""
		if s.AlbumID != "" {
			cover = "http://imge.kugou.com/stdmusic/150/" + s.AlbumID + ".jpg"
		}
		// 酷狗 JS 音源 getURL 用 hash 作 id
		out = append(out, ChartSong{
			Source: "kugou", ID: s.Hash, Title: s.SongName,
			Artist: s.SingerName, Cover: cover,
		})
	}
	return out
}

// ---- 榜单面板（独立「排行」板块，不注册为推荐插件）----
type chartPanel struct {
	fetcher chartFetcher
	mu      sync.Mutex
	lists   []chartList
	loaded    bool
	loading   bool
	activeList string // 当前选中的榜单（一次只显示一个，不堆叠）
}

func (r *chartPanel) Name() string { return r.fetcher.name() }

func (r *chartPanel) ensureLoaded(invalidate func()) {
	r.mu.Lock()
	if r.loaded || r.loading {
		r.mu.Unlock()
		return
	}
	r.loading = true
	r.mu.Unlock()
	go func() {
		lists, err := r.fetcher.fetch()
		r.mu.Lock()
		r.loading = false
		if err == nil {
			r.lists = lists
			r.loaded = true
		}
		r.mu.Unlock()
		invalidate()
	}()
}

func (r *chartPanel) snapshot() []chartList {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lists
}

func (r *chartPanel) Render(c *ui.Context, a *app, preview bool) {
	r.ensureLoaded(a.Invalidate)
	playRef := a.PlayRef
	lists := r.snapshot()
	if len(lists) == 0 {
		ui.Text(c, "加载榜单…").FontSize(12 * uiScale).TextColor(clrMuted)
		return
	}
	// 榜单下拉（模仿搜索栏音源选择：一次只显示一个榜，不堆一起）
	names := make([]string, 0, len(lists))
	for _, l := range lists {
		names = append(names, l.Name)
	}
	if r.activeList == "" {
		r.activeList = names[0]
	}
	cur := r.activeList
	ui.Row(c).AlignSelf(ui.Stretch).AlignItems(ui.Center).Gap(8 * uiScale).Children(func() {
		sel := ui.SelectBase(c, &r.activeList)
		sel.Trigger.Height(32 * uiScale).PaddingX(10 * uiScale).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(5 * uiScale).Children(func() {
				ui.Text(c, cur).FontSize(12 * uiScale).Bold().TextColor(clrText)
				ui.Text(c, "▾").FontSize(10 * uiScale).TextColor(clrMuted)
			})
		})
		popupH := len(names)*40 + 12
		if popupH > 360 {
			popupH = 360
		}
		sel.Popup(func(panel ui.Element) {
			panel.Padding(5 * uiScale).Background(clrCard).Radius(10 * uiScale).Height(float32(popupH) * uiScale)
			ui.Scroll(c).Grow(1).Scrollbars(false).Children(func() {
				for _, name := range names {
					name := name
					item := sel.Item(name).Height(40 * uiScale).PaddingX(10 * uiScale)
					if item.Highlighted() || name == cur {
						item.Background(clrCardHov)
					}
					item.Children(func() {
						ui.Text(c, name).FontSize(12 * uiScale).TextColor(clrText)
					})
				}
			})
		})
	})
	// 只渲染选中的榜
	var selected *chartList
	for i := range lists {
		if lists[i].Name == r.activeList {
			selected = &lists[i]
			break
		}
	}
	if selected == nil {
		selected = &lists[0]
	}
	limit := 20 // 详情页全量
	if preview {
		limit = 3 // 来源页预览 3 首
	}
	n := len(selected.Songs)
	if n > limit {
		n = limit
	}
	for i := 0; i < n; i++ {
		s := selected.Songs[i]
		if chip(c, fmt.Sprintf("%d. %s — %s", i+1, s.Title, s.Artist)).Clicked() {
			playRef(s.Source, s.ID, s.Title, s.Artist, s.Cover)
		}
	}
}

// 榜单面板实例（两个平台；由排行板块下拉切换，不注册为推荐插件）。
var chartPanels = []*chartPanel{
	{fetcher: neteaseChart{}},
	{fetcher: kugouChart{}},
}

