//go:build native

// stats_page.go —— 播放统计页（按歌/歌手展示，数据来自 play_stats.go）。
package main

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
	"mymusic-mygo/plugin"
)

func (a *app) statsPage(c *ui.Context) {
	ui.Scroll(c).Fill().Scrollbars(false).Children(func() {
		ui.Column(c).Gap(10 * uiScale).Padding(4*uiScale, a.resp(24, 12), 24*uiScale, a.resp(24, 12)).Children(func() {
			ui.Text(c, "播放统计").FontSize(20 * uiScale).Bold().TextColor(clrText)
			s := a.loadPlayStats()
			total := 0
			totalSec := 0.0
			for _, x := range s.Songs {
				total += x.Count
				totalSec += x.TotalSec
			}
			ui.Text(c, fmt.Sprintf("累计播放 %d 次 · 总时长 %s", total, fmtDuration(totalSec))).
				FontSize(12 * uiScale).TextColor(clrMuted)

			// 常听歌手
			artists := a.topArtists(10)
			if len(artists) > 0 {
				ui.Text(c, "常听歌手").FontSize(15 * uiScale).Bold().TextColor(clrText).Padding(8*uiScale, 0, 0, 0)
				for i, ar := range artists {
					chip(c, fmt.Sprintf("%d. %s — %d 次", i+1, ar.Name, ar.Count))
				}
			}

			// 常听歌曲（点击直接播放）
			songs := a.topSongs(20)
			if len(songs) > 0 {
				ui.Text(c, "常听歌曲").FontSize(15 * uiScale).Bold().TextColor(clrText).Padding(8*uiScale, 0, 0, 0)
				for i, x := range songs {
					x := x
					label := fmt.Sprintf("%d. %s — %s · %d 次", i+1, x.Title, x.Artist, x.Count)
					if chip(c, label).Clicked() {
						a.PlayRef(x.Source, x.ID, x.Title, x.Artist, x.Cover)
					}
				}
			}
			if len(s.Songs) == 0 {
				ui.Text(c, "还没有播放记录").FontSize(13 * uiScale).TextColor(clrMuted)
			}

			// 清空统计
			clear := ui.Box(c).AlignSelf(ui.Start).Height(34 * uiScale).PaddingX(14 * uiScale).
				Radius(8 * uiScale).Background(clrCard)
			if clear.Clicked() {
				a.savePlayStats(playStats{})
			}
			clear.Children(func() {
				ui.Text(c, "清空统计").FontSize(12 * uiScale).TextColor(clrMuted).AlignSelf(ui.Center)
			})
		})
	})
}

func fmtDuration(sec float64) string {
	d := time.Duration(sec * float64(time.Second))
	if d < time.Hour {
		return fmt.Sprintf("%d 分钟", int(d.Minutes()))
	}
	return fmt.Sprintf("%d 小时 %d 分", int(d.Hours()), int(d.Minutes())%60)
}

// 本地统计推荐插件（注册在 chart.go init 之外；和榜单并列，按启用状态显示）。
type localStatsRecommend struct{}

func (localStatsRecommend) Name() string { return "常听推荐" }

func (localStatsRecommend) Render(c *ui.Context, h plugin.RecommendHost, preview bool) {
	limit := 10
	if preview {
		limit = 3
	}
	// 通过宿主能访问的全局统计：app 是唯一 host 实现，取 topSongs。
	// 这里用类型断言拿 app（依赖反转的例外：本地统计插件与宿主同包，允许）。
	if a, ok := h.(*app); ok {
		songs := a.topSongs(limit)
		if len(songs) == 0 {
			return
		}
		ui.Text(c, "你常听的").FontSize(13 * uiScale).Bold().TextColor(clrText).Padding(0, 0, 2*uiScale, 0)
		for _, s := range songs {
			if chip(c, fmt.Sprintf("%s — %s · %d 次", s.Title, s.Artist, s.Count)).Clicked() {
				h.PlayRef(s.Source, s.ID, s.Title, s.Artist, s.Cover)
			}
		}
	}
}

func init() { plugin.RegisterRecommend(localStatsRecommend{}) }
