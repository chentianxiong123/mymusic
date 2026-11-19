//go:build native

package main

import (
	"database/sql"

	"homecast/service/conv"
)

// settingStore 插件设置存储（系统统一管理，内核服务）。
// 实现 plugin.SettingStore：包装主线 conv.SaveSetting/LoadSetting（settings 表）。
// 依赖反转：plugin 包只认接口，这里注入实现。
type settingStore struct {
	db *sql.DB
}

// Set 存设置（key 已含 plugin: 命名空间前缀）。
func (s *settingStore) Set(key, value string) error {
	if s == nil || s.db == nil {
		return nil
	}
	return conv.SaveSetting(s.db, key, value)
}

// Get 读设置；未设置/读失败 → ok=false。
func (s *settingStore) Get(key string) (string, bool) {
	if s == nil || s.db == nil {
		return "", false
	}
	return conv.LoadSetting(s.db, key)
}

// newSettingStore 创建设置存储（db 可为 nil → 内存模式）。
func newSettingStore(db *sql.DB) *settingStore {
	return &settingStore{db: db}
}
