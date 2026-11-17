#!/usr/bin/env bash
# mymusic 安卓构建脚本（主线自带，官方源码不进项目）
#
# 流程：下载官方 mygo mobile 分支源码到 ~/mygo-mobile-src → 打 android.patch
#       （JDK21/阿里云镜像/本机 Gradle）→ 重编官方 CLI（embed 生效）→ 构建安卓。
#
# 依赖（本机）：JDK21、Gradle 9.3.1（~/.gradle/wrapper/dists/）、NDK 28.2、Android SDK。
# 官方源码在 ~/mygo-mobile-src（项目外），不进 git。
set -euo pipefail

# ---- 可配置 ----
SRC_DIR="${MYGO_MOBILE_SRC:-$HOME/mygo-mobile-src}"
CLI_BIN="${MYGO_CLI_BIN:-$HOME/go/bin/mygo}"
PATCH_SCRIPT="$(cd "$(dirname "$0")" && pwd)/patch-template.sh"
REPO_GO_MOD="$HOME/go/pkg/mod"          # Go module cache
BRANCH="mobile"

log() { printf '\033[1;36m[build-android]\033[0m %s\n' "$*"; }
die() { printf '\033[1;31m[build-android] 错误:\033[0m %s\n' "$*"; exit 1; }

# ---- ① 下载官方 mobile 分支源码（浅克隆，项目外）----
if [ ! -d "$SRC_DIR/.git" ]; then
  log "浅克隆官方 mygo $BRANCH 分支 → $SRC_DIR"
  rm -rf "$SRC_DIR"
  git clone --depth 1 --branch "$BRANCH" https://github.com/egoist/mygo "$SRC_DIR" \
    || die "官方源码克隆失败（检查网络/代理）"
else
  log "官方源码已存在：$SRC_DIR"
fi

# ---- ② 打补丁（JDK21/阿里云/本机 Gradle）----
log "打补丁 → 官方模板（JDK21 + 阿里云 + 本机 Gradle 9.3.1）"
bash "$PATCH_SCRIPT" "$SRC_DIR" || die "补丁应用失败"

echo "--- 补丁后 JDK 版本（应 21）---"
grep -rn "jvmToolchain(21)" "$SRC_DIR/internal/mobile/android/mygo/build.gradle.kts" 2>/dev/null | head -2

# ---- ③ 重编官方 CLI（内嵌改好的模板）----
log "重编官方 CLI → $CLI_BIN"
cd "$SRC_DIR"
GOFLAGS=-mod=mod go build -o "$CLI_BIN" ./cmd/mygo || die "CLI 重编失败"

# ---- ④ 用 CLI 构建安卓（需在 mymusic 的 mygo 目录）----
MYGO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
log "构建安卓（CLI: $CLI_BIN，项目: $MYGO_DIR）"
cd "$MYGO_DIR"
# 确保 go.mod 引用官方 mobile 分支（替换回 v0.9.0 正式版则安卓 JNI 缺失）
if ! grep -q "replace github.com/egoist/mygo" go.mod; then
  printf '\nreplace github.com/egoist/mygo => github.com/egoist/mygo mobile\n' >> go.mod
  log "已向 go.mod 追加 replace ... mobile（安卓需要 JNI 后端）"
fi
# 关键：必须 -tags=native，否则官方 CLI（productionTags 读 GOFLAGS）编成主线 web 壳版！
# （之前踩过：漏 native → 装的是 web 壳不是 native UI）
export GOFLAGS="-mod=mod -tags=native"
export ANDROID_NDK_HOME="${ANDROID_NDK_HOME:-/usr/local/android-sdk/ndk/28.2.13676358}"
export ANDROID_NDK_ROOT="$ANDROID_NDK_HOME"
# 过滤掉本脚本自己的参数（--x86），不传给 CLI
_build_args=()
for _a in "$@"; do
  [ "$_a" = "--x86" ] || _build_args+=("$_a")
done
"$CLI_BIN" build -platform android "${_build_args[@]}"
log "构建完成：$(ls -1 "$MYGO_DIR/build/android/"*.apk 2>/dev/null | head -1)"

# ---- ⑤ 可选：重打包单 ABI（如 x86_64 给模拟器）----
# 用法：build-android.sh --x86   在构建后把 APK 重打包为只含 x86_64 的版本
pack_x86() {
  local src apk bt
  src="$(ls -1 "$MYGO_DIR/build/android/"*.apk 2>/dev/null | head -1)"
  [ -n "$src" ] || die "未找到 APK"
  bt="${ANDROID_BUILD_TOOLS:-/usr/local/android-sdk/build-tools/36.0.0}"
  apk="/tmp/mymusic-x86_64.apk"
  log "重打包 x86_64（.so 未压缩 + zipalign -p + 重签）← $src"
  rm -rf /tmp/apk-x86 && mkdir -p /tmp/apk-x86 && cd /tmp/apk-x86
  unzip -o -q "$src"
  rm -rf lib/arm64-v8a lib/armeabi-v7a
  [ -d lib/x86_64 ] || die "APK 无 x86_64 .so"
  rm -f /tmp/mymusic-x86u.apk
  # .so 必须 Stored（未压缩），否则 INSTALL_FAILED_INVALID_APK: Failed to extract native libraries
  zip -r -q -n .so /tmp/mymusic-x86u.apk . -x "META-INF/*"
  "$bt/zipalign" -p -f 4 /tmp/mymusic-x86u.apk /tmp/mymusic-x86a.apk
  "$bt/apksigner" sign --ks ~/.android/debug.keystore --ks-pass pass:android --out "$apk" /tmp/mymusic-x86a.apk
  "$bt/apksigner" verify "$apk" && log "✓ x86_64 APK 签名有效: $apk"
}

for arg in "$@"; do
  case "$arg" in
    --x86) pack_x86 ;;
  esac
done
