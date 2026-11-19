// Package conv 缓存域公共小工具：itoa / 数字字符串解析（cache.go 与 lyriccache.go
// 曾各自实现一份几乎一样代码——DRY 违规，抽到这里统一改）
package conv

import (
	"database/sql"
)

// Itoa 十进制 int → string（无 strconv import，纯手写）
func Itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 12)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}

// LoadLimit 从 settings 表读整数上限（key 所在行）；读不到或非纯数字 → 返回给定默认值
func LoadLimit(db *sql.DB, key string, def int) (int, error) {
	var v string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v); err != nil {
		return 0, err
	}
	n := 0
	for _, c := range v {
		if c < '0' || c > '9' {
			return def, nil
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

// SaveSetting 写 settings 表某键（不存在插入，存在覆盖）。settings 写入唯一公共入口——
// 各域包（cache/lyriccache/playback）曾各自 Exec 几乎一样 SQL（DRY 违规），统一收拢到
// 这里，未来表结构迁移只改这一处，调用方（含 mygo naive 端）自动跟上。
func SaveSetting(db *sql.DB, key, value string) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec(`INSERT INTO settings (key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=?`,
		key, value, value)
	return err
}

// LoadSetting 读 settings 表某键原字符串；键不存在/读失败 → ok=false。
// settings 读唯一公共入口（与 SaveSetting 对称），读取方改表结构自动跟上。
func LoadSetting(db *sql.DB, key string) (string, bool) {
	if db == nil {
		return "", false
	}
	var v string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v); err != nil {
		return "", false
	}
	return v, true
}

// ValidateTableName 表名白名单校验：只允许 [A-Za-z0-9_]，且不空、不超 64 字符。
// 用途：ClearUnfav 等场景需要拼接表名进 SQL，字符串拼接虽调用方一致，但风格危险——
// 万一将来有人传用户输入进 favTable，就是 SQL 注入。非法返回空串（调用方应终止查询）。
func ValidateTableName(name string) string {
	if name == "" || len(name) > 64 {
		return ""
	}
	for _, c := range name {
		isAlnum := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
		if !isAlnum && c != '_' {
			return ""
		}
	}
	return name
}
