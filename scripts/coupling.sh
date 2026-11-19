#!/usr/bin/env bash
# 代码耦合度检查：包依赖 fan-in/fan-out + 越层依赖告警
#
# 用法:
#   scripts/coupling.sh            # 全部（go/ 主线 + mygo/）
#   scripts/coupling.sh --main     # 只主线 go/
#   scripts/coupling.sh --mygo     # 只 mygo/
#   scripts/coupling.sh --json     # 输出原始依赖矩阵（供其他工具消费）
#
# 原理: go list -f '{{.ImportPath}} {{join .Imports ","}}' ./...
#       统计包间(项目内部)依赖的 fan-in(被依赖数)/fan-out(依赖数)，
#       并按"分层规则"检测越层/反向依赖告警。
# 不修改任何代码，只读分析。

set -u
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MODE="all"
OUT="text"
for a in "$@"; do
  case "$a" in
    --main) MODE="main" ;;
    --mygo) MODE="mygo" ;;
    --json) OUT="json" ;;
    --all)  MODE="all" ;;
    *) echo "unknown arg: $a" >&2; exit 1 ;;
  esac
done

# 收集一个模块的包依赖矩阵: "importpath|dep1,dep2,..."
collect() { # $1=模块根目录  $2=内部前缀
  local dir="$1" prefix="$2"
  ( cd "$dir" && go list -f '{{.ImportPath}}|{{join .Imports ","}}' ./... 2>/dev/null ) \
    | grep "^$prefix" || true
}

MAIN_DEPS="$(collect "$ROOT/go" homecast)"
MYGO_DEPS="$(collect "$ROOT/mygo" mymusic-mygo)"

if [ "$OUT" = "json" ]; then
  printf '%s\n' "$MAIN_DEPS" "$MYGO_DEPS" | grep -v '^$'
  exit 0
fi

# 越层/反向依赖规则: "依赖方|被依赖方|说明"
RULES=$(cat <<'EOF'
mymusic-mygo/lyric|mymusic-mygo/source|歌词域依赖音源域: LyricLine 类型放错包, 应抽共享底层(如 model)
mymusic-mygo/source|mymusic-mygo/lyric|音源域依赖歌词域: 反向耦合, 不应存在
mymusic-mygo/source|mymusic-mygo/target|音源域依赖目标域: 越层
mymusic-mygo/target|mymusic-mygo/source|目标域依赖音源域: 越层
mymusic-mygo/plugin|mymusic-mygo|插件域依赖根包: 子包不应依赖组合根
mymusic-mygo/lyric|mymusic-mygo|歌词域依赖根包: 子包不应依赖组合根
mymusic-mygo/source|mymusic-mygo|音源域依赖根包: 子包不应依赖组合根
mymusic-mygo/target|mymusic-mygo|目标域依赖根包: 子包不应依赖组合根
homecast/service/|homecast/web|业务域依赖 web 层: 越层
homecast/service/|homecast/server|业务域依赖 server 层: 越层
homecast/service/|homecast/api|业务域依赖 api 层: 越层
homecast/service/|homecast/hx|业务域依赖 hx 层: 越层
EOF
)

DATA="$(printf '%s\n%s\n' "$MAIN_DEPS" "$MYGO_DEPS" | grep -v '^$')"
python3 - "$RULES" "$DATA" <<'PY'
import sys, collections, re

data = sys.argv[2]
rules_raw = sys.argv[1]
lines = [l for l in data.splitlines() if '|' in l]

deps = {}
for line in lines:
    pkg, imp = line.split('|', 1)
    deps[pkg] = [x for x in imp.split(',') if x and not x.startswith(('.', 'log'))]

# 只统计项目内部依赖（homecast / mymusic-mygo）
def internal(x):
    return x.startswith('homecast') or x.startswith('mymusic-mygo')

fanin = collections.Counter()
for pkg, ds in deps.items():
    for d in ds:
        if internal(d):
            fanin[d] += 1

print("=" * 62)
print("包间耦合 fan-in（被依赖数）: 高 = 耦合核心/热点")
print("=" * 62)
for pkg, n in sorted(fanin.items(), key=lambda kv: -kv[1]):
    if n >= 2:
        print(f"  {n:3d}  {pkg}")

print()
print("=" * 62)
print("越层 / 反向依赖告警")
print("=" * 62)
warn = 0
for rule in rules_raw.splitlines():
    rule = rule.strip()
    if not rule or '|' not in rule:
        continue
    a, b, msg = rule.split('|', 2)
    # 支持 homecast/service/ 这类前缀匹配
    for pkg, ds in sorted(deps.items()):
        for d in ds:
            hit_a = pkg.startswith(a) if a.endswith('/') else pkg == a
            hit_b = d.startswith(b) if b.endswith('/') else d == b
            if hit_a and hit_b:
                warn += 1
                print(f"  [越层] {pkg}  →  {d}")
                print(f"         原因: {msg}")

if warn == 0:
    print("  (无)")
else:
    print(f"\n  共 {warn} 处越层/反向依赖")

print()
print("=" * 62)
print(f"包总数 {len(deps)}  内部依赖边 {sum(len([d for d in ds if internal(d)]) for ds in deps.values())}")
print("=" * 62)
PY
