# mymusic-mygo —— MyGo 框架实现（主发行线）

用 **[MyGo](https://mygo.run)**（Go 优先桌面/移动框架，**无 cgo**）实现的音乐播放器。
**无 cgo** 是核心：`CGO_ENABLED=0` 任意机器交叉编译（Linux 交叉出 macOS 实测可行）。

> 原为「试水切片」，实测验证关键痛点被 MyGo 直接消解后**升级为主发行线**（wails3 线另见仓库根 README）。

## 核心特性

1. **无 cgo 桌面/移动**：原生 UI 一行选项做无边框+透明+置顶+鼠标穿透；`CGO_ENABLED=0` 交叉编译。
2. **B站 DASH 音频原生解码**：`audio` 包 + `audio/aac` 直接解 `mp4a.40.2`，无软转码秒开。
3. **多音源插件体系**：10 个官方 JS 插件（kugou/kuwo/migu/netease/qianqian/qqmusic/audius/ccmixter/jamendo/archive）+ 系统级 Go 音源/词源/目标。
4. **智能歌词**：匹配（时长重排 + 标题打分）、繁简折叠、定时歌词（音源自带 LRC）。
5. **更新机制**：用户自填源（GitHub/自建 URL/代理）+ ed25519 签名 manifest，公钥构建期烙进二进制。

## 端 / 构建（CLI 钉 v0.10.0）

| 端 | 命令 | 产物 |
|---|---|---|
| 🐧 Linux | `mygo build -platform linux/amd64` | deb/rpm/tar.gz/PKGBUILD |
| 🪟 Windows | `mygo build -platform windows/amd64` | exe |
| 🍎 macOS | `mygo build -platform darwin/arm64` | .app + dmg + manifest |
| 🤖 Android | `mygo build -platform android` | apk（通用 3 ABI）+ aab |
| 📱 iOS | `mygo build -platform ios` | **仅 macos+Xcode**（Swift 宿主，见 docs）|

> **v0.10.0 起** CLI 支持移动平台（`mobilePlatform`）。此前 v0.9.x 无 iOS/Android。
> Android 实测：Linux 直编 2m28s（JDK17 + NDK28.2 + `-tags=native`）。
> iOS：**Swift 宿主硬墙**（swiftc 仅 Xcode）→ 必须 macos runner；黑路（osxcross/Darling/graphics.gd）全判死。

## 插件体系（多音源）

```
mygo/plugin/
├── builtin.go     # 系统级能力清单（source/lyric/target 唯一事实来源，注册期防覆盖）
├── jsplugin.go    # JS 插件加载/校验（CheckPluginKey 防覆盖系统 key）
└── plugins/       # 10 个官方 JS 插件（音源/词源/目标）
```

- **系统级 = Go 实现**（恒定基线，永远在链尾）：B站/本地音源 + 网易云词源 + local/cast/bluetooth/speaker 目标
- **插件 = 同源词优先候选**（链头，零匹配风险天然对齐），失败自动降级系统词
- 保护规则：`CheckPluginKey` 校验命中系统 key 即拒（JS 插件不能静默覆盖系统能力）

## 更新机制（设置 → 关于）

- 源：GitHub owner/name 或自建 URL 根 + 代理，**运行期自填**（持久化 settings 表）
- 签名：**ed25519 公钥构建期烙进二进制**（用户不能自填，防后门）；manifest = SHA256(资产) 签名
- 移动端：**"发现新版本"→ 跳浏览器下载**（系统安装器升级保数据）；桌面："安装并重启"
- 开关：**"禁止检查更新"** 持久化（`mygo_upddisable`）

## 歌词匹配（服务端 `go/service/lyric`）

- `rankCandidates`：时长主（±2s/±5s/±10s）+ 标题次（相同100/包含80/共享2字60）+ 网易云相关度兜底
- `coreTitle` 归一（剥括号 + 剥离版本音质词）；`CleanSearchKeyword` 繁简折叠（opencc-go）
- "对齐≈匹配"发现：同源音频+同源 LRC 天然对齐（残留 +0.2~+0.8s，全落 ±1s）

## 运行

```bash
go tool mygo dev        # 开发（live reload）
go run .                # 或直接跑
MYGO_DEMO=1 go run .    # 自检：自动搜索→播放→开歌词挂件
```

测试（需网络）：`go test -tags native ./...`（**必须 `-tags native`**，否则编译失败）

## 结构

```
mymusic-mygo/
├── main.go            # app 状态 + 主窗口 view
├── player.go          # 搜索/播放/进度 goroutine + MYGO_DEMO 自检
├── lyric_bridge.go    # 歌词桥（插件词优先 → 系统网易云 fallback）
├── update_source.go   # 更新机制（GitHub/自建URL/代理 + ed25519 验签）
├── plugin/            # 插件体系（系统清单 + JS 插件加载）
├── bilibili/          # B站协议客户端（纯标准库，复用自 go/internal/bilibili）
└── android/           # 安卓签名 keystore + 构建脚本
```

## 详细文档

- 构建约束/方案：`../docs/development/2026-11-21-mygo-iOS-Android构建-约束与方案.md`
- 歌词对齐调研：`../docs/development/2026-11-20-歌词时间对齐调研与B站字幕可达性.md`
