// Package model 跨层共享的领域类型。放主线包（homecast/model），
// 因为主线域服务（playback/cast/speaker/streamsupply）与 mygo 子模块都要用，
// 依赖方向只能从 mygo → homecast，不能反过来。
package model

// SongRef 跨音源歌曲引用：所有层之间传这个，不再传裸 songID/bvid。
//
// 规则：
//   - Source = 音源名（"bilibili"/"netease"/"qqmusic"/"local"/插件 key）
//   - ID     = 该音源内的唯一 id（B站=bvid，网易云=数字 id，本地=路径）
//   - URL    = 已知直链（可选；本地源 file://，或已解析的直链）
//
// 同一首歌从搜索→播放→歌词→输出目标全程携带，切换输出目标标识不变。
type SongRef struct {
	Source string `json:"source"`
	ID     string `json:"id"`
	Title  string `json:"title"`
	Artist string `json:"artist"`
	Cover  string `json:"cover"`
	URL    string `json:"url,omitempty"`
}

// Valid 是否可用于取流。
func (r SongRef) Valid() bool {
	return r.Source != "" && r.ID != ""
}
