//go:build native

// applog.go —— 应用日志系统：捕获全部 log.Printf 输出，结构化存储（级别/分类/
// 时间），同时写内存环形缓冲（设置页查看）+ 持久化文件（落 app 数据目录）。
// 配置（最低级别/容量/写文件）持久化 settings，清空按钮真清内存+截断文件。
package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"homecast/service/conv"
)

// LogLevel 日志级别（数值越大越严重）。
type LogLevel int

const (
	logDebug LogLevel = iota
	logInfo
	logWarn
	logError
)

func (l LogLevel) String() string {
	switch l {
	case logDebug:
		return "DEBUG"
	case logInfo:
		return "INFO"
	case logWarn:
		return "WARN"
	case logError:
		return "ERROR"
	}
	return "?"
}

var logLevelNames = []string{"DEBUG", "INFO", "WARN", "ERROR"}

// LogEntry 一条结构化日志。
type LogEntry struct {
	Time     time.Time
	Level    LogLevel
	Category string
	Message  string
}

// appLogger 全局日志器：线程安全环形缓冲 + 文件 + 配置。
type appLogger struct {
	mu        sync.Mutex
	entries   []LogEntry
	capacity  int
	minLevel  LogLevel
	writeFile bool
	filePath  string
	file      *os.File
	stderr    io.Writer // 原始 stderr，日志仍输出到终端（便于 nohup 外部查看）
	categories map[string]bool // 已见分类（下拉筛选）
}

var appLog = &appLogger{
	capacity:  1000,
	minLevel:  logInfo,
	writeFile: true,
	stderr:    os.Stderr,
}

// logTimePrefix 匹配 log 标准输出的时间前缀 "2026/11/24 18:32:11 "。
var logTimePrefix = regexp.MustCompile(`^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2} `)

// logCatPrefix 匹配分类标签 "[lyric] " / "[mygo-output] " 等。
var logCatPrefix = regexp.MustCompile(`^\[([^\]]+)\] `)

// initAppLog 启动时调用：读配置 → 打开文件 → 接管 log.SetOutput。
// 必须在所有 log.Printf 之前调用（main 最开头）。
func initAppLog(dbPath string, cfg logConfig) {
	appLog.mu.Lock()
	appLog.capacity = cfg.Capacity
	appLog.minLevel = cfg.MinLevel
	appLog.writeFile = cfg.WriteFile
	appLog.filePath = filepath.Join(filepath.Dir(dbPath), "app.log")
	appLog.mu.Unlock()

	if appLog.writeFile {
		f, err := os.OpenFile(appLog.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err == nil {
			appLog.mu.Lock()
			appLog.file = f
			appLog.mu.Unlock()
		}
	}
	log.SetOutput(appLog)
	log.SetFlags(log.LstdFlags) // 保留标准时间前缀（Write 里解析）
}

// logConfig 持久化配置。
type logConfig struct {
	Capacity  int
	MinLevel  LogLevel
	WriteFile bool
}

const (
	appLogCfgKey       = "mygo_logcfg"
	appLogCapacityKey  = "mygo_logcap"
	appLogLevelKey     = "mygo_loglevel"
	appLogWriteFileKey = "mygo_logfile"
)

// restoreLogConfig 从 settings 恢复日志配置（db 就绪后调）。
func (a *app) restoreLogConfig() {
	if a.db == nil {
		return
	}
	cfg := logConfig{Capacity: 1000, MinLevel: logInfo, WriteFile: true}
	if v, ok := conv.LoadSetting(a.db, appLogCapacityKey); ok {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n >= 50 && n <= 10000 {
			cfg.Capacity = n
		}
	}
	if v, ok := conv.LoadSetting(a.db, appLogLevelKey); ok {
		for i, name := range logLevelNames {
			if v == name {
				cfg.MinLevel = LogLevel(i)
			}
		}
	}
	if v, ok := conv.LoadSetting(a.db, appLogWriteFileKey); ok {
		cfg.WriteFile = v != "0"
	}
	appLog.configure(cfg)
}

func (l *appLogger) configure(cfg logConfig) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.capacity = cfg.Capacity
	l.minLevel = cfg.MinLevel
	l.writeFile = cfg.WriteFile
	// 容量缩小：裁掉最旧的
	if len(l.entries) > l.capacity {
		l.entries = l.entries[len(l.entries)-l.capacity:]
	}
	if cfg.WriteFile && l.file == nil && l.filePath != "" {
		if f, err := os.OpenFile(l.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			l.file = f
		}
	}
}

func (a *app) saveLogConfig() {
	if a.db == nil {
		return
	}
	l := appLog
	l.mu.Lock()
	cap, lvl, wf := l.capacity, l.minLevel.String(), l.writeFile
	l.mu.Unlock()
	_ = conv.SaveSetting(a.db, appLogCapacityKey, fmt.Sprintf("%d", cap))
	_ = conv.SaveSetting(a.db, appLogLevelKey, lvl)
	if wf {
		_ = conv.SaveSetting(a.db, appLogWriteFileKey, "1")
	} else {
		_ = conv.SaveSetting(a.db, appLogWriteFileKey, "0")
	}
}

// Write 实现 io.Writer：解析 log 输出 → 结构化存储 + 文件 + stderr。
func (l *appLogger) Write(p []byte) (int, error) {
	raw := strings.TrimRight(string(p), "\r\n")
	if raw == "" {
		return len(p), nil
	}
	// 解析时间前缀
	t := time.Now()
	body := raw
	if m := logTimePrefix.FindString(raw); m != "" {
		if parsed, err := time.ParseInLocation("2006/01/02 15:04:05", strings.TrimSpace(m), time.Local); err == nil {
			t = parsed
		}
		body = raw[len(m):]
	}
	// 显式级别前缀（logExplicit 输出 "[INFO][cat] msg"）
	lvl := LogLevel(-1)
	for i, name := range logLevelNames {
		prefix := "[" + name + "]"
		if strings.HasPrefix(body, prefix) {
			lvl = LogLevel(i)
			body = strings.TrimSpace(body[len(prefix):])
			break
		}
	}
	// 解析分类
	cat := "general"
	if m := logCatPrefix.FindStringSubmatch(body); m != nil {
		cat = m[1]
		body = body[len(m[0]):]
	}
	// 推断级别（无显式前缀时）
	if lvl < 0 {
		lvl = inferLogLevel(body)
	}

	l.mu.Lock()
	l.entries = append(l.entries, LogEntry{Time: t, Level: lvl, Category: cat, Message: body})
	if len(l.entries) > l.capacity {
		l.entries = l.entries[len(l.entries)-l.capacity:]
	}
	if l.categories == nil {
		l.categories = map[string]bool{}
	}
	l.categories[cat] = true
	shouldWriteFile := l.writeFile && lvl >= l.minLevel && l.file != nil
	var f *os.File
	if shouldWriteFile {
		f = l.file
	}
	stderr := l.stderr
	l.mu.Unlock()

	// 文件写入（带级别/分类前缀，便于外部 grep）
	if f != nil {
		fmt.Fprintf(f, "%s [%s] [%s] %s\n", t.Format("2006/01/02 15:04:05"), lvl, cat, body)
	}
	// 原始输出照旧到 stderr（nohup 外部可见）
	if stderr != nil {
		fmt.Fprintln(stderr, raw)
	}
	return len(p), nil
}

// inferLogLevel 从消息内容推断级别（兼容现有 log.Printf 无显式级别的调用）。
func inferLogLevel(msg string) LogLevel {
	lower := strings.ToLower(msg)
	if strings.Contains(msg, "失败") || strings.Contains(msg, "错误") ||
		strings.Contains(lower, "error") || strings.Contains(lower, "fatal") ||
		strings.Contains(lower, "panic") {
		return logError
	}
	if strings.Contains(msg, "跳过") || strings.Contains(msg, "警告") ||
		strings.Contains(msg, "降级") || strings.Contains(lower, "warn") ||
		strings.Contains(lower, "skip") {
		return logWarn
	}
	return logInfo
}

// logExplicit 显式级别日志（新代码用，避免依赖关键词推断）。
// 格式：[LEVEL][cat] msg → Write 解析 cat 后，级别从 "[DEBUG]" 前缀识别。
func logExplicit(lvl LogLevel, cat, format string, args ...any) {
	log.Printf("[%s][%s] %s", lvl, cat, fmt.Sprintf(format, args...))
}

// logEntries 返回当前日志快照（按时间正序），供 UI 筛选。
func logEntries() []LogEntry {
	appLog.mu.Lock()
	defer appLog.mu.Unlock()
	out := make([]LogEntry, len(appLog.entries))
	copy(out, appLog.entries)
	return out
}

// logCategories 返回已见分类（下拉筛选）。
func logCategories() []string {
	appLog.mu.Lock()
	defer appLog.mu.Unlock()
	out := make([]string, 0, len(appLog.categories))
	for k := range appLog.categories {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// logStats 返回容量/当前条数/级别/文件开关/文件路径（UI 状态行）。
func logStats() (cap, n int, lvl LogLevel, wf bool, path string) {
	appLog.mu.Lock()
	defer appLog.mu.Unlock()
	return appLog.capacity, len(appLog.entries), appLog.minLevel, appLog.writeFile, appLog.filePath
}

// logClear 真清空：清内存环形缓冲 + 截断持久化文件（不是只清 UI）。
func logClear() error {
	appLog.mu.Lock()
	defer appLog.mu.Unlock()
	appLog.entries = nil
	if appLog.file != nil {
		if err := appLog.file.Truncate(0); err != nil {
			return err
		}
		if _, err := appLog.file.Seek(0, io.SeekStart); err != nil {
			return err
		}
	}
	if appLog.filePath != "" {
		// 文件句柄未打开（写文件关闭时）也要截断，保证「清空」真的清了
		if err := os.Truncate(appLog.filePath, 0); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// logSetCapacity / logSetLevel / logSetWriteFile 配置项修改（UI 调用）。
func logSetCapacity(n int) {
	appLog.mu.Lock()
	if n < 50 {
		n = 50
	}
	if n > 10000 {
		n = 10000
	}
	appLog.capacity = n
	if len(appLog.entries) > n {
		appLog.entries = appLog.entries[len(appLog.entries)-n:]
	}
	appLog.mu.Unlock()
}

func logSetLevel(l LogLevel) {
	appLog.mu.Lock()
	appLog.minLevel = l
	appLog.mu.Unlock()
}

func logSetWriteFile(on bool) {
	appLog.mu.Lock()
	appLog.writeFile = on
	if on && appLog.file == nil && appLog.filePath != "" {
		if f, err := os.OpenFile(appLog.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			appLog.file = f
		}
	}
	appLog.mu.Unlock()
}

// logExport 导出当前日志到指定路径（UI 调系统下载目录）。
func logExport(dst string) error {
	entries := logEntries()
	var buf bytes.Buffer
	for _, e := range entries {
		fmt.Fprintf(&buf, "%s [%s] [%s] %s\n", e.Time.Format("2006/01/02 15:04:05"), e.Level, e.Category, e.Message)
	}
	return os.WriteFile(dst, buf.Bytes(), 0o644)
}
