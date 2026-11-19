package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"mymusic-mygo/source"
)

// localSource 本地文件音源：扫配置目录 mp3/flac/m4a 听本机歌（免登录、零版权、无网络）。
// 无歌词（GetLyric nil）；播放 GetStream 返回 file:// URL（mygoOutput 直接读文件，不下载）。
// 目录由设置注入（main.go 读「本地音乐目录」→ newLocalSource(dir)）。
type localSource struct {
	dir string
}

func newLocalSource(dir string) *localSource { return &localSource{dir: dir} }

func (s *localSource) Name() string { return "local" }

// Search 扫目录，文件名匹配 keyword（子串，不区分大小写）→ 分页。
func (s *localSource) Search(keyword string, page, size int) ([]source.Song, int, error) {
	files, err := s.scan()
	if err != nil {
		return nil, 0, err
	}
	var songs []source.Song
	kw := strings.ToLower(strings.TrimSpace(keyword))
	for _, f := range files {
		base := strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))
		if kw != "" && !strings.Contains(strings.ToLower(base), kw) {
			continue
		}
		songs = append(songs, source.Song{
			ID:       f, // 绝对路径（GetStream file:// 用）
			Title:    base,
			Artist:   "",
			Cover:    "",
			Duration: "",
			Typename: "local",
			Album:    "",
		})
	}
	total := len(songs)
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	return songs[start:end], total, nil
}

// GetStream 本地文件直链（file:// 前缀，mygoOutput 特殊处理直接读文件）。
func (s *localSource) GetStream(id string, quality int) (*source.Stream, error) {
	if _, err := os.Stat(id); err != nil {
		return nil, err
	}
	return &source.Stream{URL: "file://" + id, Quality: quality}, nil
}

// GetLyric 本地文件无歌词（nil → 播放歌词静默，UI 不报错）。
func (s *localSource) GetLyric(id string, keywords []string, sid int) ([]source.LyricLine, error) {
	return nil, nil
}

// scan 扫目录音频文件（mp3/flac/m4a/ogg/wav/aac；递归）。
func (s *localSource) scan() ([]string, error) {
	if s.dir == "" {
		return nil, nil
	}
	var out []string
	_ = filepath.WalkDir(s.dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".mp3", ".flac", ".m4a", ".ogg", ".wav", ".aac":
			out = append(out, path)
		}
		return nil
	})
	sort.Strings(out)
	return out, nil
}
