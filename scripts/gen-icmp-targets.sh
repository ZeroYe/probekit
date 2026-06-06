#!/usr/bin/env bash
# ============================================================
# ICMP 目标批量生成脚本
# 用法:
#   ./gen-icmp-targets.sh < ip_list.txt > icmp_targets.yaml
#   ./gen-icmp-targets.sh --cidr 10.0.0.0/24 > icmp_targets.yaml
#   ./gen-icmp-targets.sh --csv targets.csv > icmp_targets.yaml
# ============================================================
set -euo pipefail

# ---- 默认值 ----
INTERVAL="60s"
COUNT="4"
TIMEOUT="5s"
# 全局标签（所有目标共用）
declare -A GLOBAL_LABELS=()

# ---- 解析参数 ----
MODE="stdin"
CIDR=""
CSV=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --cidr)  MODE="cidr"; CIDR="$2"; shift 2 ;;
    --csv)   MODE="csv";  CSV="$2"; shift 2 ;;
    --interval) INTERVAL="$2"; shift 2 ;;
    --count)  COUNT="$2";  shift 2 ;;
    --timeout) TIMEOUT="$2"; shift 2 ;;
    --label)
      # --label region=cn-beijing
      key="${2%%=*}"
      val="${2#*=}"
      GLOBAL_LABELS["$key"]="$val"
      shift 2
      ;;
    -h|--help)
      echo "用法: $0 [选项] < ip_list.txt"
      echo "  --cidr <CIDR>        从 CIDR 段生成"
      echo "  --csv <文件>         从 CSV 文件读取（列: ip[,interval,count,timeout,labels]）"
      echo "  --interval <值>      默认探测间隔 (默认: 60s)"
      echo "  --count <值>         默认发包数 (默认: 4)"
      echo "  --timeout <值>       默认超时 (默认: 5s)"
      echo "  --label K=V          全局标签，可多次指定"
      echo ""
      echo "示例:"
      echo "  # 从文件读取 IP 列表"
      echo "  cat ips.txt | $0 > /etc/probekit/icmp_targets.yaml"
      echo ""
      echo "  # CIDR 段生成，带标签"
      echo "  $0 --cidr 192.168.1.0/24 --label region=cn-beijing --label rack=A01 > targets.yaml"
      echo ""
      echo "  # CSV 文件，每行独立配置"
      echo "  $0 --csv targets.csv > targets.yaml"
      echo ""
      echo "  # 结合使用: CIDR 生成 + CSV 补充"
      echo "  $0 --cidr 10.0.0.0/24 > base.yaml"
      echo "  $0 --csv special.csv >> base.yaml"
      exit 0
      ;;
    *)
      echo "未知参数: $1"
      exit 1
      ;;
  esac
done

# ---- 生成 YAML ----
emit_target() {
  local host="$1"
  local interval="${2:-$INTERVAL}"
  local count="${3:-$COUNT}"
  local timeout="${4:-$TIMEOUT}"
  local extra_labels="$5"  # 格式: key1=val1,key2=val2

  echo "  - host: \"$host\""
  echo "    interval: $interval"
  echo "    count: $count"
  echo "    timeout: $timeout"

  # 合并全局标签 + 本目标标签
  local merged_labels=()
  for key in "${!GLOBAL_LABELS[@]}"; do
    merged_labels+=("$key=${GLOBAL_LABELS[$key]}")
  done

  IFS=',' read -ra extra <<< "$extra_labels"
  for pair in "${extra[@]}"; do
    if [[ -n "$pair" ]]; then
      merged_labels+=("$pair")
    fi
  done

  if [[ ${#merged_labels[@]} -gt 0 ]]; then
    echo "    labels:"
    for pair in "${merged_labels[@]}"; do
      key="${pair%%=*}"
      val="${pair#*=}"
      echo "      $key: \"$val\""
    done
  fi

  echo ""
}

# CIDR 转 IP 列表
cidr_to_ips() {
  python3 -c "
import ipaddress, sys
net = ipaddress.ip_network('$CIDR', strict=False)
for ip in net.hosts():
    print(ip)
" 2>/dev/null || {
    # 回退: 用 nmap 或 awk
    which nmap &>/dev/null && {
      nmap -sL -n "$CIDR" 2>/dev/null | grep "Nmap done" -A 999 | tail -n +2 | awk '{print $NF}' | grep -E '^[0-9]'
    }
  }
}

# CSV 格式: ip,interval,count,timeout,labels
# labels 格式: key1=val1|key2=val2
read_csv() {
  local file="$1"
  # 跳过表头
  tail -n +2 "$file" | while IFS=',' read -r ip interval count timeout labels rest; do
    # labels 中的 | 转回逗号
    labels="${labels//|/,}"
    emit_target "$ip" "$interval" "$count" "$timeout" "$labels"
  done
}

# ---- 主流程 ----
echo "# 由 gen-icmp-targets.sh 自动生成"
echo "# 生成时间: $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
echo ""

case "$MODE" in
  cidr)
    cidr_to_ips | while read -r ip; do
      [[ -z "$ip" ]] && continue
      emit_target "$ip"
    done
    ;;
  csv)
    read_csv "$CSV"
    ;;
  stdin)
    # 从 /dev/stdin 读取 IP，一行一个
    while IFS= read -r ip; do
      [[ -z "$ip" || "$ip" == \#* ]] && continue
      emit_target "$ip"
    done
    ;;
esac
