// 歌单功能切片：歌单列表页 / 详情页 / 增删改（htmx 片段替换）
// 与 playlist（= 播放队列）区分：歌单是用户自建、可命名、可多份的收藏集合
package hx

import (
	"net/http"
	"strconv"

	"homecast/service/songlist"
)

// songlistPageData 歌单列表页数据
type songlistPageData struct {
	Lists []songlist.Songlist
}

// songlistDetailData 歌单详情页数据
type songlistDetailData struct {
	Songlist songlist.Songlist
	Items    []songlist.Item
}

// SonglistsPage GET /hx/songlists → 歌单列表页
func (h *H) SonglistsPage(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, "songlists", "content_songlists.html", &songlistPageData{Lists: h.Songlist.List()})
}

// SonglistDetailPage GET /hx/songlists/{id} → 歌单详情页
func (h *H) SonglistDetailPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	g, items, err := h.Songlist.Get(id)
	if err != nil {
		http.Error(w, "歌单不存在", http.StatusNotFound)
		return
	}
	h.renderPage(w, "songlists", "content_songlist_detail.html", &songlistDetailData{Songlist: g, Items: items})
}

// SonglistCreate POST /hx/songlists/create → 新建（返回网格片段替换 #songlist-grid）
func (h *H) SonglistCreate(w http.ResponseWriter, r *http.Request) {
	if _, err := h.Songlist.Create(r.FormValue("name")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.renderSonglistGrid(w)
}

// SonglistRename POST /hx/songlists/rename/{id} → 改名（返回网格片段）
func (h *H) SonglistRename(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := h.Songlist.Rename(id, r.FormValue("name")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.renderSonglistGrid(w)
}

// SonglistDelete POST /hx/songlists/delete/{id} → 删除（返回网格片段）
func (h *H) SonglistDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := h.Songlist.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.renderSonglistGrid(w)
}

// SonglistAddItem POST /hx/songlists/add/{id} → 加一首（返回歌曲列表片段）
//
// 参数走 query（对齐 fav_btn.html：安卓壳 wails3 asset 桥丢弃 POST body）
func (h *H) SonglistAddItem(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	sec, _ := strconv.Atoi(r.FormValue("duration"))
	_, err = h.Songlist.AddSong(id, songlist.Item{
		BVID:        r.FormValue("bvid"),
		Title:       r.FormValue("title"),
		Artist:      r.FormValue("artist"),
		Cover:       r.FormValue("cover"),
		Duration:    r.FormValue("durationText"),
		DurationSec: sec,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.renderSonglistItems(w, id)
}

// SonglistRemoveItem POST /hx/songlists/remove/{id}/{bvid} → 移除（返回歌曲列表片段）
func (h *H) SonglistRemoveItem(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	h.Songlist.RemoveSong(id, r.PathValue("bvid"))
	h.renderSonglistItems(w, id)
}

// SonglistClear POST /hx/songlists/clear/{id} → 清空（歌单保留，返回歌曲列表片段）
func (h *H) SonglistClear(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	h.Songlist.Clear(id)
	h.renderSonglistItems(w, id)
}

// renderSonglistGrid 渲染歌单网格片段（共用零件）
func (h *H) renderSonglistGrid(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.Tpl.ExecuteTemplate(w, "songlist_grid.html", &songlistPageData{Lists: h.Songlist.List()})
}

// renderSonglistItems 渲染歌单内歌曲片段（共用零件）
func (h *H) renderSonglistItems(w http.ResponseWriter, id int64) {
	g, items, err := h.Songlist.Get(id)
	if err != nil {
		http.Error(w, "歌单不存在", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.Tpl.ExecuteTemplate(w, "songlist_items.html", &songlistDetailData{Songlist: g, Items: items})
}
