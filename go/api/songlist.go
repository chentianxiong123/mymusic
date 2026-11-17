package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"homecast/service/songlist"
)

// SonglistHandler 歌单端点（歌单是资源，有 id）
//
//	GET    /api/v1/songlists                    → 歌单列表
//	POST   /api/v1/songlists                    → 新建 {name}
//	POST   /api/v1/songlists/{id}/rename        → 改名 {name}
//	DELETE /api/v1/songlists/{id}               → 删除（级联条目）
//	GET    /api/v1/songlists/{id}/items         → 歌单 + 歌曲
//	POST   /api/v1/songlists/{id}/items         → 加一首 {bvid,title,...}
//	DELETE /api/v1/songlists/{id}/items/{bvid}  → 移除一首
//	POST   /api/v1/songlists/{id}/clear         → 清空（歌单保留）
type SonglistHandler struct {
	Songlist *songlist.SonglistService
}

// pathID 解析 {id} 路径参数（非数字 → 0，调用方按不存在处理）
func pathID(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id
}

// List GET /api/v1/songlists
func (h *SonglistHandler) List(w http.ResponseWriter, r *http.Request) {
	ok(w, h.Songlist.List())
}

// Create POST /api/v1/songlists  body: {name}
func (h *SonglistHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errResp(w, http.StatusBadRequest, "invalid body: name required")
		return
	}
	g, err := h.Songlist.Create(body.Name)
	if err != nil {
		errResp(w, http.StatusBadRequest, err.Error())
		return
	}
	ok(w, g)
}

// Rename POST /api/v1/songlists/{id}/rename  body: {name}
//
// 用 POST 而非 PATCH：对齐项目现有路由风格（router_test 白名单只允许 GET/POST/DELETE）
func (h *SonglistHandler) Rename(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errResp(w, http.StatusBadRequest, "invalid body: name required")
		return
	}
	if err := h.Songlist.Rename(pathID(r), body.Name); err != nil {
		songlistErr(w, err)
		return
	}
	ok(w, true)
}

// Delete DELETE /api/v1/songlists/{id}
func (h *SonglistHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.Songlist.Delete(pathID(r)); err != nil {
		songlistErr(w, err)
		return
	}
	ok(w, true)
}

// Get GET /api/v1/songlists/{id}/items
func (h *SonglistHandler) Get(w http.ResponseWriter, r *http.Request) {
	g, items, err := h.Songlist.Get(pathID(r))
	if err != nil {
		songlistErr(w, err)
		return
	}
	ok(w, map[string]any{"songlist": g, "items": items})
}

// AddItem POST /api/v1/songlists/{id}/items  body: songlist.Item
func (h *SonglistHandler) AddItem(w http.ResponseWriter, r *http.Request) {
	var it songlist.Item
	if err := json.NewDecoder(r.Body).Decode(&it); err != nil || it.BVID == "" {
		errResp(w, http.StatusBadRequest, "invalid body: bvid required")
		return
	}
	added, err := h.Songlist.AddSong(pathID(r), it)
	if err != nil {
		songlistErr(w, err)
		return
	}
	ok(w, map[string]any{"added": added})
}

// RemoveItem DELETE /api/v1/songlists/{id}/items/{bvid}
func (h *SonglistHandler) RemoveItem(w http.ResponseWriter, r *http.Request) {
	bvid := r.PathValue("bvid")
	if bvid == "" {
		errResp(w, http.StatusBadRequest, "bvid required")
		return
	}
	ok(w, h.Songlist.RemoveSong(pathID(r), bvid))
}

// Clear POST /api/v1/songlists/{id}/clear
func (h *SonglistHandler) Clear(w http.ResponseWriter, r *http.Request) {
	h.Songlist.Clear(pathID(r))
	ok(w, true)
}

// songlistErr 域错误 → HTTP 状态（ErrNotFound=404，ErrEmptyName=400，其余 500）
func songlistErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, songlist.ErrNotFound):
		errResp(w, http.StatusNotFound, "歌单不存在")
	case errors.Is(err, songlist.ErrEmptyName):
		errResp(w, http.StatusBadRequest, "歌单名不能为空")
	default:
		errResp(w, http.StatusInternalServerError, err.Error())
	}
}
