# HomeMusic

<p align="center">
  <b>🎵 用 Go 写的多端音乐播放器 · 多音源接入（B站 / 网易云 / 本地 + JS 插件）</b>
  <br><small>两条实现线并重：<b>mygo</b>（Go 原生框架，无 cgo，主发行线）与 <b>wails3</b>（原多端壳，htmx + SQLite）。</small>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.27-00ADD8?logo=go" alt="Go">
  <img src="https://img.shields.io/badge/mygo-v0.10.0-3B3B5C?logo=go" alt="mygo">
  <img src="https://img.shields.io/badge/wails3-beta.25-777BB4?logo=wails" alt="wails3">
  <img src="https://img.shields.io/badge/SQLite-内嵌-003B57?logo=sqlite" alt="SQLite">
  <img src="https://img.shields.io/badge/多音源-B站%2F网易云%2F本地-FF6C2C" alt="多音源">
</p>

---

## 两条实现线（都重要）

### ① mygo —— Go 原生框架（主发行线，无 cgo）

| 能力 | 说明 |
|---|---|
| 端 | 🐧 Linux · 🪟 Windows · 🍎 macOS · 🤖 Android · 📱 iOS |
| 特点 | **无 cgo**（`CGO_ENABLED=0` 任意机器交叉编译）；原生 UI（无边框/透明/置顶/鼠标穿透一行搞定）；B站 DASH 音频原生解码 |
| 多音源 | B站 / 网易云 / 本地 + **JS 插件体系**（10 个官方插件：kugou/kuwo/migu/netease/qianqian/qqmusic/audius/ccmixter/jamendo/archive）|
| 歌词 | 智能识别 + 时长重排 + 标题打分匹配；统一繁简折叠；定时歌词（音源自带 LRC）|
| 更新 | 用户自填源（GitHub / 自建 URL / 代理）+ ed25519 签名 manifest，公钥构建期烙进二进制 |
| 构建 | `mygo build -platform <goos/goarch>`（v0.10.0 起含 iOS/Android）；CLI 钉 v0.10.0 |

构建命令：
```bash
cd mygo && mygo build -platform linux/amd64          # Linux（deb/rpm/tar.gz）
cd mygo && mygo build -platform windows/amd64        # Windows
cd mygo && mygo build -platform darwin/arm64         # macOS（.app + dmg）
cd mygo && mygo build -platform android              # 安卓（JDK17 + NDK28.2，实测 2m28s）
cd mygo && mygo build -platform ios                  # iOS（仅 macos + Xcode，Swift 宿主）
```

### ② wails3 —— 原多端壳（htmx + SQLite）

| 能力 | 说明 |
|---|---|
| 端 | 🐧 Linux · 🪟 Windows · 🤖 Android |
| 特点 | 一个壳 `wails3/` 三端构建；htmx 服务端渲染 + Tailwind + SQLite 全内嵌；桌面歌词挂件（GTK/Win32，cgo）|
| 构建 | `cd wails3 && ./build-linux.sh` · `./build-windows.sh` · `task android:package` |
| 挂件 | ✅ GTK3 透明穿透在用 · 🟡 Win32 就位待真机 · 🔲 安卓悬浮窗待写 |

> 挂件是 cgo（GTK/Win32 是 C 库）——语言边界，不是取舍。

---

## 架构演进：Python 探索 → Go 实现

核心技术路线 **「Python 探索、Go 实现」**：

- **`explore/`（历史只读，不维护）** —— Python/Vue 老版 + 手搓安卓壳，归档
- **`go/`** —— 业务核心（wails3 线）：htmx + SQLite 单二进制，三端共用
- **`mygo/`** —— mygo 线：Go 原生框架应用（无 cgo），独立发行
- **`wails3/`** —— wails3 统一壳（原多端）
- Python 端沉淀的接口语义（`/api/v1/*` 前缀、`{code,message,data}` 信封）已 1:1 对齐进 Go 实现

---

## 功能

- 🎧 **B站音乐** — 搜索 / 收藏 / 播放 / 本地歌单
- 🎵 **多音源接入** — B站 / 网易云 / 本地 + JS 插件（kugou/kuwo/migu/qianqian/qqmusic/audius/ccmixter/jamendo/archive）
- 📝 **智能歌词** — 匹配（时长重排 + 标题打分）、繁简折叠、定时歌词（音源自带 LRC）
- 🪟 **桌面歌词挂件** — 主界面跟随歌词 + 桌面穿透挂件（wails3 线，GTK/Win32）
- 📺 **DLNA 投屏** — 自动发现设备，一键投屏（wails3 线）
- 🔊 **小米音箱** — 登录 / 设备管理 / 音乐推送（wails3 线）
- 🔄 **更新机制** — 用户自填源 + ed25519 签名（mygo 线）

---

## 项目结构

```
mymusic/
├── mygo/        # 主发行线：Go 原生框架应用（无 cgo）+ JS 插件 + 更新机制 + 移动端构建
├── go/          # wails3 线业务核心：cmd/server + web（htmx）+ internal（api/域包/store）
├── wails3/      # wails3 统一壳（三端）+ 挂件多态 + 构建脚本
├── explore/     # 历史只读：legacy/（Python+Vue 老版）/ android/（手搓壳）
├── docs/        # 排查/架构记录（troubleshooting 风格）
├── scripts/     # E2E 冒烟等
└── .github/     # CI（全手动 workflow_dispatch）：mygo + wails 每端独立配置
```

## CI（全手动触发）

- `.github/workflows/` 每端×每方法独立配置，**全部 `workflow_dispatch`，零自动触发**
- mygo：test / linux / windows / android / macos(白+黑) / ios(白) / web
- wails3：linux / windows / macos / ios / android / harmony / wechat / e2e + backend
- 签名：安卓固定 `release.keystore`（覆盖安装保数据）；桌面/更新 ed25519（私钥=secret，防后门）

---

## License

MIT
