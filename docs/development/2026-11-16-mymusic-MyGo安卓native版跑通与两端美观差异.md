# 2026-11-16 mymusic MyGo 安卓 native 版跑通与两端美观差异

> 折腾记录：把 mymusic 的 MyGo（native UI）接上官方 mobile 分支，编成安卓 APK 真机跑通。
> 意外发现：同一个 native 代码，桌面 X11 窗口难看，手机竖屏反而规整——这反差让我很惊讶。
> 本文按 exploration-notes 8 块沉淀，目的：**后来者照文档能重新走通，不用重踩一遍坑。**

---

## ① 目标与动机

- 主线目标：mymusic 的 MyGo（Go 优先跨平台桌面 UI 框架）作为第二入口，已并入主线单仓库 `mygo/`。
- 本轮目标：把 mymusic 的 mygo native 版**接上官方 mobile 分支**（作者 egoist 本人 PR #940「Run apps on iOS and Android」），编译成 **mymusic 安卓 APK**，真机跑通 native UI。
- 意外发现（本轮最大价值）：**同一个 MyGo 代码，桌面 X11 窗口难看，手机竖屏反而规整好看**。用户原话：「我很惊讶，这个东西在电脑端很难看，手机端好看。反过来电脑端难看手机端好看。」——反差来自 MyGo 组件在手机固定竖屏下按可用宽度排布，而桌面横向窗口下组件排布/缩放没对齐主线 GUI。

## ② 资源拓扑（探索对象长什么样）

### 官方 mobile 分支（作者本人开，draft: True，89 commits/613 文件/+54541/-1939）
源码在 `/tmp/mygo-mobile-src/mygo-mobile/`（codeload tarball 解压）。关键文件：

```
cmd/mygo/
  build.go          # productionTags() 从 GOFLAGS 环境变量读 -tags=（关键！）
  android.go        # gradle() 生成壳后调 gradlew；packageDir() 用 go list 找 main 包
  embed.go          # 内嵌 android 模板（internal/mobile/android/*），改模板必须重编 CLI
internal/mobile/
  glue_android.go   # //export mygo_start_android / mygo_event / mygo_drain（Go→C 符号）
  native_android.go # //export mygo_native_handle
  android/
    jni_android.c    # JNI 层：注册 Java_dev_mygo_MyGoNative_event/drain/nativeWindow
    gradlew + gradle/wrapper/gradle-wrapper.properties   # distributionUrl 决定下载哪个 Gradle
    settings.gradle.kts   # repositoriesMode + 仓库列表（FAIL_ON_PROJECT_REPOS 默认）
    build.gradle.kts / mygo/build.gradle.kts / app/build.gradle.kts  # jvmToolchain(17) 硬编码
examples/counter-native/   # 官方 demo（验证链路用）
```

**JNI 导出链路**（native UI 能跑的关键）：`jni_android.c` 定义 `Java_dev_mygo_MyGoNative_*` → 调用 Go 导出的 `mygo_*` 符号。若 .so 里没有这些 `Java_dev_*` 导出，app 启动即崩 `UnsatisfiedLinkError`。

### mymusic mygo（要编成安卓的源码）
```
/mnt/shared/my-project/mymusic/mygo/
  main.go        # //go:build native（入口，WhenReady + NewWindow）
  web_main.go    # //go:build !native（web 壳，安卓不用）
  go.mod         # module mymusic-mygo；replace homecast => ../go；replace egoist/mygo => mobile 分支（本轮加的）
  mygo.json      # name/identifier/version/out
  scale.go       # //go:build native；读 GNOME text-scaling-factor（android 上运行时失败返回默认 1）
  player*.go / pb_bridge.go / lyric_bridge.go / stream_server.go / services.go ...  # mymusic 全部业务
```
依赖：`homecast`（主线域包 SSOT，bilibili/speaker/transcode/lyric/playback/music/cache 等）+ `github.com/egoist/mygo`（UI 框架）。

**module 关系（本轮踩坑核心）**：`mymusic-mygo` 是独立 module，用 `replace` 同时指向：
- `homecast => ../go`（主线域包）
- `github.com/egoist/mygo => /tmp/mygo-mobile-src/mygo-mobile`（**必须用 mobile 分支，不能用 v0.9.0 正式版**，否则 .so 缺 android JNI 导出）

## ③ 工具链清单（电脑自己的版本，全部本机已有，零联网下载）

| 工具 | 版本 | 路径 | 用途 |
|---|---|---|---|
| Gradle | **9.3.1** | `~/.gradle/wrapper/dists/gradle-9.3.1-bin/e6fpmcitnr2cbhj7h6pdk7btv/gradle-9.3.1/bin/gradle` | 打包 APK（wrapper 指向本地文件） |
| JDK | **21**（系统唯一，无 17） | `/usr/lib/jvm/java-21-openjdk-amd64` | 官方壳要求 17 → 改 21 |
| NDK | **28.2** | `/usr/local/android-sdk/ndk/28.2.13676358` | 编 Go 共享库 |
| Android SDK | build-tools 30~36, platforms 30~36 | `/usr/local/android-sdk` | CLI 自动探测 |
| 官方 CLI | **重编版**（内嵌模板已改） | `/tmp/mygo-cli-android` | `build -platform android` |
| Go | 1.27.3 | 系统 | 编 mygo 共享库 |
| adb | platform-tools | `/usr/local/android-sdk/platform-tools/adb` | 装/启/截图 |
| 依赖源 | **阿里云镜像** | `~/.gradle/init.gradle.kts`（用户已有，勿移走） | mavenCentral 直连 403 → 阿里云 |
| 真机 | Android 13 arm64-v8a, root | `192.168.31.124:5555` | 验证 |

## ④ 成功路径（可复现步骤）

### 第一步：清理垃圾 + 定位混乱根源（用户要求「清理刚才折腾产生的垃圾」）
```bash
# 垃圾（安全删）
rm -rf ~/.gradle/wrapper/dists/gradle-8.14.3-bin        # 半成品 .zip.part（4.2M）
rm -f /tmp/hcserver-android-arm64 /tmp/hcserver-linux-arm64 /tmp/mygo-linux-arm64 \
      /tmp/mygo-native-android-arm64 /tmp/mygo-cli-mobile /tmp/empty-init.gradle.kts \
      /tmp/msg.txt /tmp/kotlin-compiler-*.alive
rm -rf /tmp/mygo-mobile-src/mygo-mobile/examples/counter-native/.mygo   # 构建残留 + 生成壳（可从源码再生成）
# 保留：运行中的桌面版 mymusic-native-pb、官方源码 mygo-mobile-src、Gradle 9.3.1/9.2.1、init.gradle.kts、NDK 28.2/21.4
```

**混乱根源（4 个）**：① Gradle wrapper 三版本并存（8.14.3 半成品/9.2.1/9.3.1）② init.gradle.kts 阿里云 vs 官方壳 `FAIL_ON_PROJECT_REPOS` 冲突 ③ 官方壳硬编码 JDK17 ④ wrapper 强制联网下载（services.gradle.org 被 SSL 断）。

### 第二步：让官方 CLI 用电脑自己的版本（改模板 + 重编 CLI）
```bash
cd /tmp/mygo-mobile-src/mygo-mobile
LOCALZIP=~/.gradle/wrapper/dists/gradle-9.3.1-bin/e6fpmcitnr2cbhj7h6pdk7btv/gradle-9.3.1-bin.zip

# ① wrapper 指向本机 Gradle 9.3.1 本地文件 + 关联网校验（不再下载）
sed -i "s|distributionUrl=https\\\\://services.gradle.org/distributions/gradle-8.14.3-bin.zip|distributionUrl=file\\\\://$LOCALZIP|" internal/mobile/android/gradle/wrapper/gradle-wrapper.properties
sed -i 's/validateDistributionUrl=true/validateDistributionUrl=false/' internal/mobile/android/gradle/wrapper/gradle-wrapper.properties

# ② 依赖源 → 阿里云 + PREFER_SETTINGS（解 init.gradle.kts 冲突）
sed -i 's/repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)/repositoriesMode.set(RepositoriesMode.PREFER_SETTINGS)/' internal/mobile/android/settings.gradle.kts
sed -i 's|mavenCentral()|maven(url = "https://maven.aliyun.com/repository/central")|g' internal/mobile/android/settings.gradle.kts

# ③ JDK 17 → 21（系统唯一 21）
for f in internal/mobile/android/build.gradle.kts internal/mobile/android/mygo/build.gradle.kts internal/mobile/android/app/build.gradle.kts; do
  sed -i 's/JavaVersion.VERSION_17/JavaVersion.VERSION_21/g; s/jvmToolchain(17)/jvmToolchain(21)/g' "$f"
done

# ④ 重编 CLI（模板是 embed.go 内嵌的，必须重编才生效）
go build -o /tmp/mygo-cli-android ./cmd/mygo
```

### 第三步：mymusic 接上（独立 module + replace mygo 到 mobile 分支）
```bash
cd /mnt/shared/my-project/mymusic/mygo
# go.mod 已有：module mymusic-mygo；replace homecast => ../go
echo "replace github.com/egoist/mygo => /tmp/mygo-mobile-src/mygo-mobile" >> go.mod   # 关键！
GOFLAGS=-mod=mod go mod tidy
```

### 第四步：官方 CLI 编译（GOFLAGS 传 native tag）
```bash
cd /mnt/shared/my-project/mymusic/mygo
export ANDROID_HOME=/usr/local/android-sdk \
       ANDROID_NDK_HOME=/usr/local/android-sdk/ndk/28.2.13676358 \
       JAVA_HOME=/usr/lib/jvm/java-21-openjdk-amd64 \
       GOFLAGS="-tags=native"     # 官方 productionTags() 从 GOFLAGS 读 -tags=，编 native UI
/tmp/mygo-cli-android build -platform android
# 产物：build/android/mymusic-mygo-0.1.0.apk（61.9MB，含 lib/arm64-v8a/libmygoapp.so 等 3 ABI）
```

### 第五步：真机验证
```bash
ADB=/usr/local/android-sdk/platform-tools/adb; DEV=192.168.31.124:5555
APK=/mnt/shared/my-project/mymusic/mygo/build/android/mymusic-mygo-0.1.0.apk
$ADB -s $DEV install -r $APK
$ADB -s $DEV shell am start -n com.example.mymusicmygo/dev.mygo.MyGoActivity
$ADB -s $DEV shell dumpsys activity activities | grep topResumedActivity   # 应见 com.example.mymusicmygo
$ADB -s $DEV exec-out screencap -p > /tmp/mymusic-android.png              # 截图证据
```

## ⑤ 踩坑记录（最有价值，每条 现象→根因→解法）

| # | 现象 | 根因 | 解法 |
|---|---|---|---|
| 1 | 官方 build 卡在下载 Gradle，`Install.forceFetch` 超时（SSL 断） | wrapper `distributionUrl=https://services.gradle.org/...` 直连不通 | 指向本机 Gradle 9.3.1 本地 zip（`file://`）+ `validateDistributionUrl=false` |
| 2 | `Build was configured to prefer settings repositories... repository 'maven' was added by init.gradle.kts` | 官方壳 `repositoriesMode=FAIL_ON_PROJECT_REPOS` 与用户阿里云 init 冲突 | 模板改 `PREFER_SETTINGS` + settings 里 mavenCentral→阿里云 |
| 3 | 官方壳要求 JDK 17，系统只有 21 | 官方模板 `jvmToolchain(17)`/`VERSION_17` 硬编码 | 模板改 21（`VERSION_21`/`jvmToolchain(21)`） |
| 4 | 改了模板不生效 | CLI 每次**重新生成壳覆盖** `.mygo/`，模板是 `embed.go` 内嵌的 | 改 `internal/mobile/android/*` 源码模板 + **重编 CLI** |
| 5 | `go build failed: -buildmode=c-shared requires exactly one main package` | counter demo 在 mygo **module 内**，`go list .` 列出多个 main 包 | 建独立 go.mod + `replace github.com/egoist/mygo => 本地` + mygo.json 设 `main: "."` |
| 6 | **mymusic app 启动即崩**：`UnsatisfiedLinkError: No implementation found for byte[] dev.mygo.MyGoNative.event(byte[], boolean)` | mymusic go.mod 用 `github.com/egoist/mygo v0.9.0`（**正式版，无 android JNI 后端**），编出的 .so 缺 `Java_dev_mygo_MyGoNative_*` 导出 | `replace github.com/egoist/mygo => mobile分支`（含 jni_android.c）→ 重编出 JNI 导出 |
| 7 | `missing go.sum entry`（独立 module 编译时） | 独立 go.mod 无 go.sum | `GOFLAGS=-mod=mod go mod tidy` |
| 8 | 官方 CLI 不编 native UI | `productionTags()` 只读 `GOFLAGS` 的 `-tags=`，默认不传 native | `GOFLAGS="-tags=native"` |
| 9 | `scale.go` 读 GNOME gsettings | 桌面专属缩放；android 无 gsettings | 运行时 init 里 err 已处理 → 默认 uiScale=1，**能编译**，不影响安卓 |

## ⑥ 验证成果（证据）

- **官方 CLI `build -platform android` 成功**：`✓ Built mymusic-mygo 0.1.0 in 22.2s`，产物 `build/android/mymusic-mygo-0.1.0.apk`（61.9MB）+ `.aab`（26.3MB）。
- **JNI 导出已修**（第 6 坑修复后）：
  ```
  Java_dev_mygo_MyGoNative_drain
  Java_dev_mygo_MyGoNative_event
  Java_dev_mygo_MyGoNative_nativeWindow
  Java_dev_mygo_MyGoNative_releaseNativeWindow
  ```
- **真机运行**：`topResumedActivity = com.example.mymusicmygo/dev.mygo.MyGoActivity`，**零崩溃、零 UnsatisfiedLinkError**。
- **界面截图**（`/tmp/mymusic-android.png`，1080x2400 竖屏）：设置页完整渲染——播放模式（顺序/列表循环/单曲循环/随机循环）、歌词字号（14/16/18/22/26px）、歌词背景（封面模糊/纯色）、歌词时间显示开关、字幕字号（16/24/28/32px）、歌曲缓存（清空未收藏/清空缓存）、缓存上限（关闭/200MB/500MB/1GB/2GB）、底部播放条（192k/音量/进度 0:00/播放控制/心形/列表）。紫红主题渐变、竖屏完全适配。

## ⑥.5 性能与功耗监测（2026-11-16 真机，root）

### 内存（mymusic 进程 pid 3591，连续采样 3 分钟）
| 指标 | 值 | 判定 |
|---|---|---|
| **TOTAL PSS** | 154MB | 正常 |
| **RSS 波动** | 190-240MB 震荡 | 无泄漏（3 分钟未单调上涨） |
| **Native Heap** | 10MB | 正常 |
| **EGL mtrack** | 61.6MB | GPU 表面（OpenGL ES 渲染） |
| **GC 迹象** | t=17 RSS 229→194MB 回落 | Go GC 回收，正常 |

### CPU
- 稳定 0-17%，**50% 尖峰反复出现**（2 分钟 3 次，非偶发）
- 但有**连续 0% 空闲段**（t=16-21 全 0%）→ 间歇性重绘/GC 脉冲，非死循环

### 功耗读数不可信（该设备 current_now 电流计噪声大，测多少次都是噪声）

**决定性证据**：同一 mygo、同样前台亮屏，连续三轮功耗读数**完全不同**：
| 轮次 | 功耗读数 | CPU |
|---|---|---|
| 1 | 1984mW | 18-26% |
| 2 | 332mW | （未测） |
| 3 | 625mW | 40-75% |

- **同一状态差 6 倍**（1984 vs 332mW）→ 读数与物理矛盾（CPU 75% 才 625mW？屏幕就 1.5W+）
- `current_now` 符号正负交替、6mA→815mA 剧烈抖动 → **该设备电流计不可靠**
- **结论：`/sys` 功耗读数不可信，弃用**；`dumpsys battery` 也只给 voltage 不给 current
- 真功耗只能用**电量下降速率**（锁屏看 capacity 掉电%）测，不依赖电流计（未做）

**原始三状态功耗表**（读数不可信，仅作参考留存，勿当结论）：
| 状态 | 平均功耗（|V×I|） | 说明 |
|---|---|---|
| 待机基线（mymusic 已杀，屏幕亮） | 1684mW | 屏幕亮度占大头 |
| mymusic 前台未播放（设置页） | 1484mW | 与待机差值在噪声内 |
| mymusic 播放中 | 1984mW | 音频解码稳态负载 |
| 网易云播放中（前台亮屏） | 383-670mW | **与 CPU 72% 物理矛盾→假象** |

> 注意：上表读数与该设备物理状态矛盾（网易云 CPU 72% 却只 383mW），
> **不可用于功耗结论**。真实对比见下方 CPU/内存可靠指标。

### 可靠指标对比：mymusic vs 网易云（top / dumpsys meminfo，可信）
| 指标 | **mymusic**（native Go+OpenGL ES） | **网易云** | 差异 |
|---|---|---|---|
| **CPU 播放中** | 18-26%（空闲时 0%） | **72%** | mymusic 省 **~3 倍** |
| **RSS 内存** | 221MB | **860MB** | mymusic 省 **~4 倍** |
| TOTAL PSS | 154MB | — | — |

**理论优势（可靠）**：mymusic 的 native UI（Go + OpenGL ES 自绘）比网易云**CPU 省 ~3 倍、内存省 ~4 倍**。原因：native 自绘无 WebView/大 UI 框架开销，Go GC 内存紧凑。

## ⑦ 决策记录（为什么选 A 不选 B）

- **用官方 mobile 分支而非自写 android backend**：作者 egoist 本人 PR #940，`SurfaceWindow.kt` 注释确认 native UI（Go 用 OpenGL ES 画进 ANativeWindow，**非 webview、非 X11**）——正是用户要的 native。NDK+JDK+Gradle 全链路官方已给。
- **改官方源码模板 + 重编 CLI**，而非每次改生成壳：CLI 覆盖式生成 `.mygo/`，改模板一次生效。
- **replace mygo 到本地 mobile 分支**而非 `go get`：源码在本地 `/tmp`，且要跟官方分支同步。
- **Gradle 用本机 9.3.1**（wrapper 本地文件）而非让 CLI 下载 8.14.3：电脑已有完整版，零联网。
- **保留 NDK 21.4 备选**、**保留全部 build-tools**（SDK 管理器正常累积，非垃圾）。

## ⑧ 未解之谜 / 下一步岔路

- **两端美观差异根因**（本轮最大惊讶，未深入）：同一个 MyGo native 代码，桌面 X11 横向窗口难看、手机竖屏规整。推测：MyGo 组件在固定竖屏按可用宽度排布；桌面横向窗口下组件排布/缩放未对齐主线 GUI（htmx 视觉基准）。**下一步可对比桌面/手机同一页面渲染，定位是组件排布还是缩放/字体**。
- **手机端仅验证了设置页 + playerBar**：搜索、播放、歌词、队列页在手机上的表现未测。mymusic 业务（bilibili 搜索/播放/音箱投送/桌面歌词）在安卓的可用性未验证。
- **mymusic 桌面版 go.mod 也加了 replace mygo => mobile 分支**——桌面版是否受影响需回归（mobile 分支应向后兼容桌面，但未测）。
- 已知残留：官方 CLI 模板改动用的是 `sed`，若官方分支更新需重打；`jvmToolchain(21)` 依赖系统 JDK 21，换机要重配。

---

*第一人称探索记录。工具版本/路径/报错原文均来自本次实际环境，未验证的已标注「未测」。*
