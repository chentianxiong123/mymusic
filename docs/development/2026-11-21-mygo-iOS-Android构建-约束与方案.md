# mygo iOS / Android 构建：约束与方案（2026-11-21）

> 结论先行：**iOS 能建但必须 macos（Swift 宿主硬墙）；Android v0.10.0 直编已在 Linux 实测跑通。**

## 0. 关键背景

- mygo 的 iOS/Android 宿主 = **Swift（iOS）/ Kotlin（Android）**，由 **CLI（`github.com/egoist/mygo/cmd/mygo`）** 构建。
- **v0.10.0（2026-11-17 发布）起**，CLI 原生支持移动平台：`mobilePlatform()` 识别 `ios`/`ios/simulator`/`android`/`android/<arch>`（`cmd/mygo/ios.go` + `android.go`）。
- **v0.10.0 之前**（v0.9.x）CLI `buildPlatforms` 只认 `darwin/linux/windows/web`，**无 iOS/Android**——移动端构建不可用。
- 因此：**`mygo/go.mod` + `.github/actions/setup-mygo` 必须钉 v0.10.0**（升级 commit `89c8cb2`）。

## 1. iOS：必须 macos + Xcode（Swift 硬墙）

构建链（`cmd/mygo/ios.go`）：
```go
// Swift host (internal/mobile/ios) + Go c-archive
// GOOS=ios needs cgo to link, with Xcode's clang
// CLI compiles, bundles, icon via actool, signs — as Xcode would.
if runtime.GOOS != "darwin" { return nil, errors.New("iOS apps build on a Mac with Xcode") }
```
- **`newIOSToolchain` 硬检查 `runtime.GOOS != "darwin"` → 报错**。
- 需要 `xcrun`（Xcode 的 clang + iOS SDK）→ **必须 macos-latest runner**。

### iOS 黑路全判死（2026-11-21 调查，勿再尝试）

| 路 | 结论 |
|---|---|
| **osxcross**（Linux 交叉）| ❌ 无 swiftc（swiftc 仅 Xcode）|
| **Swift SDK Generator**（苹果官方）| ❌ 只生成 **Linux 目标** SDK，`make-linux-sdk` 无 iOS |
| **Darling**（macOS 兼容层）| 🟡 swiftc CLI 能跑（编 macOS ✅），但 **iOS 目标未验证**、**xcodebuild 挂**（#488/#1414）、赌性大 |
| **graphics.gd**（Go iOS on Linux）| 🟡 工具链可行（Zig .tbd + ld64.lld + Go/cgo 宿主），但 mygo 需**换 Swift 宿主为 Go/cgo 壳**（工程量大）|
| **两仪**（Android APK on iOS）| ❌ 非原生（Android-in-iOS 翻译层）|

**结论：iOS 唯一可靠 = macos-latest runner（白路，`.github/workflows/mygo-ios-runner.yml`）**。

### iOS 签名（白路唯一剩的坎）

```go
// ios.go:866 "Make one for the identifier in Xcode (Settings › Accounts) or at
//         developer.apple.com, and set ios.team, or ios.provisioningProfile"
```
- **真机 `.ipa`**：需 Apple Developer（`ios.team` + provisioning profile）——**没有账号出不来真机包**。
- **模拟器**（`-platform ios/simulator`）：免签（`signForSimulator` ad-hoc），只跑模拟器。

## 2. Android：v0.10.0 直编（Linux 实测跑通）

**背景**：上游 `egoist/mygo` 的 **`mobile` 分支已删**（GitHub 查无此 ref）→ 旧 `mygo/android/build-android.sh`（克隆 mobile 分支打补丁）**废弃**。

**新打法**（`mygo build -platform android`，v0.10.0 CLI 自带 `internal/mobile/android`）：

| 项 | 值 |
|---|---|
| JDK | **17**（模板 `jvmToolchain(17)`）——Debian 13 无 openjdk-17，装 Temurin 17.0.21.1 |
| NDK | 28.2.13676358（`ANDROID_NDK_HOME`）|
| 关键 flag | `GOFLAGS=-mod=mod -tags=native`（漏 native 会编成 web 壳）|
| 签名 | 默认 debug key → workflow 用 `release.keystore` 重签（apksigner）|
| 产物 | `.apk`（通用 3 ABI：arm64-v8a/armeabi-v7a/x86_64，~100MB）+ `.aab`（Google Play 自动拆）|

**本地实测**（2026-11-21）：2m28s → `.apk` 100.3MB + `.aab` 39MB，versionCode 100 / targetSdk 36。

### Android 踩坑记录

1. **`mobile` 分支已删** → 旧 build-android.sh 必挂，CI 改 v0.10.0 直编（commit `52adb20`）。
2. **`~/.gradle/init.gradle.kts` aliyun 镜像冲突**：原文件用 `allprojects { repositories { aliyun } }`（项目级），撞模板 `RepositoriesMode.FAIL_ON_PROJECT_REPOS` → 改**设置级**（`settingsEvaluated { dependencyResolutionManagement.repositories { aliyun } }`）。
3. **JDK17 缺失**：模板 `jvmToolchain(17)`，Debian 13 默认只有 JDK21 → 装 Temurin 17。
4. **ABI 取舍**：通用（3 ABI）全覆盖 ~100MB；只编 `arm64-v8a`（`mygoAbis`）可降到 ~40-50MB。

## 3. CI 相关（全手动触发）

- `.github/workflows/mygo-android.yml`：v0.10.0 直编 + JDK17 + NDK28.2 + release.keystore 重签。
- `.github/workflows/mygo-ios-runner.yml`：白路（macos-latest + Xcode + `-platform ios`）。
- `mygo-ios-osxcross.yml`（灰路 PoC）已删（2026-11-21，用户拍板）。
- 全部 `workflow_dispatch`，零自动触发。
