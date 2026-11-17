#!/usr/bin/env bash
# 官方 mygo mobile 安卓模板「环境适配」补丁（sed 版，按字符串匹配，不依赖行号）
#
# 适配 3 处环境差（官方硬编码 JDK17/mavenCentral/联网 wrapper → 本机环境）：
#   ① JDK 17 → 21      （根/mygo/app 三个 build.gradle.kts：VERSION_17 + jvmToolchain(17)）
#   ② Maven 源         （settings.gradle.kts：FAIL_ON_PROJECT_REPOS→PREFER_SETTINGS + mavenCentral→阿里云）
#   ③ Gradle wrapper   （gradle-wrapper.properties：联网 8.14.3 → 本机 9.3.1 + validateDistributionUrl=false）
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

echo "[patch-template] ✓ 完成。验证："
grep -rn "jvmToolchain(21)" "$A/build.gradle.kts" "$A/mygo/build.gradle.kts" "$A/app/build.gradle.kts" 2>/dev/null | head -3
grep -n "PREFER_SETTINGS\|aliyun" "$A/settings.gradle.kts" 2>/dev/null | head -2
grep -n "distributionUrl\|validateDistributionUrl" "$A/gradle/wrapper/gradle-wrapper.properties" 2>/dev/null
