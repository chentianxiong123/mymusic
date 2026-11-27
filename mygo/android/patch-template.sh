#!/usr/bin/env bash
# 官方 mygo mobile 安卓模板「环境适配」补丁（sed 版，按字符串匹配，不依赖行号）
#
# 适配 5 处（官方硬编码 JDK17/mavenCentral/联网 wrapper/禁明文/fling 吞手指 → 本机需要）：
#   ① JDK 17 → 21      （根/mygo/app 三个 build.gradle.kts：VERSION_17 + jvmToolchain(17)）
#   ② Maven 源         （settings.gradle.kts：FAIL_ON_PROJECT_REPOS→PREFER_SETTINGS + mavenCentral→阿里云）
#   ③ Gradle wrapper   （gradle-wrapper.properties：联网 8.14.3 → 本机 9.3.1 + validateDistributionUrl=false）
#   ④ 明文 HTTP 放行   （cmd/mygo/android.go 的 <application> 加 android:usesCleartextTraffic="true"）
#      —— 安卓悬浮歌词 Service（Kotlin HttpURLConnection）连本机 Go 后端 http://127.0.0.1:28977，
#         Android 9+ 默认禁明文；wails3 早已踩过此坑（domain-config 不匹配 IP 字面量，
#         必须用 base-config 或 usesCleartextTraffic）。不加则歌词挂件连不上、永远默认文字。
#   ⑤ 触摸滑动惯性   （ui/touch.go：fling 中按下新手指不再 swallow 整根手指）
#      —— mygo 官方设计：手指按下停止 fling 后 swallow=true 吞掉该手指全部 move/up，
#         快速往返滑动时第二下手指只"刹车"不滚动，要抬手再按，滞后感明显。
#         改法：停止 fling 后不 swallow，继续走正常按压/滚动路径，新手指立即接管 pan。
#
# 用法：patch-template.sh <官方源码根>   （对 <根>/internal/mobile/android 就地修改）
# 幂等：可重复执行（sed 替换已是目标值则无副作用）。
set -euo pipefail

ROOT="${1:?用法: patch-template.sh <官方源码根>}"
A="$ROOT/internal/mobile/android"
[ -d "$A" ] || { echo "错误: 找不到 $A"; exit 1; }

# 本机 Gradle 9.3.1 本地 zip（wrapper distributionUrl 指向它，避免联网下载）
GRADLE_ZIP="${MYGO_GRADLE_ZIP:-$HOME/.gradle/wrapper/dists/gradle-9.3.1-bin/e6fpmcitnr2cbhj7h6pdk7btv/gradle-9.3.1-bin.zip}"
[ -f "$GRADLE_ZIP" ] || { echo "警告: 本机 Gradle zip 不存在 $GRADLE_ZIP（构建会尝试联网）"; }

echo "[patch-template] ① JDK 17→21"
for f in build.gradle.kts mygo/build.gradle.kts app/build.gradle.kts; do
  [ -f "$A/$f" ] && sed -i 's/VERSION_17/VERSION_21/g; s/jvmToolchain(17)/jvmToolchain(21)/g' "$A/$f"
done

echo "[patch-template] ② Maven 源 → 阿里云 + PREFER_SETTINGS"
[ -f "$A/settings.gradle.kts" ] && {
  sed -i 's/FAIL_ON_PROJECT_REPOS/PREFER_SETTINGS/g' "$A/settings.gradle.kts"
  # mavenCentral() 在 pluginManagement 与 dependencyResolutionManagement 两处
  sed -i 's|mavenCentral()|maven { url = uri("https://maven.aliyun.com/repository/central") }|g' "$A/settings.gradle.kts"
}

echo "[patch-template] ③ Gradle wrapper → 本机 9.3.1"
if [ -f "$A/gradle/wrapper/gradle-wrapper.properties" ]; then
  sed -i "s|^distributionUrl=.*|distributionUrl=file\\\\://$GRADLE_ZIP|" "$A/gradle/wrapper/gradle-wrapper.properties"
  sed -i 's/validateDistributionUrl=true/validateDistributionUrl=false/' "$A/gradle/wrapper/gradle-wrapper.properties"
fi

echo "[patch-template] ④ 明文 HTTP 放行（悬浮歌词 SSE 连 127.0.0.1:28977）"
ANDROID_GO="$ROOT/cmd/mygo/android.go"
if [ -f "$ANDROID_GO" ] && ! grep -q 'usesCleartextTraffic' "$ANDROID_GO"; then
  sed -i 's|android:enableOnBackInvokedCallback="true"|android:enableOnBackInvokedCallback="true"\n        android:usesCleartextTraffic="true"|' "$ANDROID_GO"
fi

echo "[patch-template] ⑤ 触摸滑动：fling 中按下新手指不再 swallow"
TOUCH_GO="$ROOT/ui/touch.go"
if [ -f "$TOUCH_GO" ] && ! grep -q 'do not swallow the finger' "$TOUCH_GO"; then
  python3 - "$TOUCH_GO" <<'PY'
import sys
p=sys.argv[1]
s=open(p).read()
old='''		if rt.flinging.on {
			// A finger stops content flinging, and taps nothing.
			rt.flinging.on = false
			t.swallow = true
			return true
		}'''
new='''		if rt.flinging.on {
			// Stop the fling, but do not swallow the finger: let it pan at once
			// (upstream swallowed it, making quick back-and-forth swipes lag).
			// Leave waiting false so a pure tap stays inert — taps nothing.
			rt.flinging.on = false
			rt.requestFrame()
			return true // swallow a new finger: no click, but pan works
		}'''
assert old in s, 'touch.go fling block not found'
open(p,'w').write(s.replace(old,new))
PY
fi

echo "[patch-template] ✓ 完成。验证："
grep -rn "jvmToolchain(21)" "$A/build.gradle.kts" "$A/mygo/build.gradle.kts" "$A/app/build.gradle.kts" 2>/dev/null | head -3
grep -n "PREFER_SETTINGS\|aliyun" "$A/settings.gradle.kts" 2>/dev/null | head -2
grep -n "distributionUrl\|validateDistributionUrl" "$A/gradle/wrapper/gradle-wrapper.properties" 2>/dev/null
grep -n "usesCleartextTraffic" "$ANDROID_GO" 2>/dev/null
grep -n "do not swallow the finger" "$TOUCH_GO" 2>/dev/null
