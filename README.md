# ProbeKit

一个轻量级网络监控代理，支持 ICMP Ping、DNS 查询和 SNMP 轮询，输出到 VictoriaMetrics，并提供 MCP (Model Context Protocol) 服务以支持 AI 辅助运维。

## 功能特性

- **ICMP 探测** — 多包 Ping，含 RTT 直方图 (p50/p90/p99)、抖动、标准差、丢包率、连续丢包计数，支持 2000+ 目标（持久化 socket + 信号量并发控制）
- **DNS 探测** — A/AAAA/MX/NS/CNAME/TXT 查询，每个目标可指定独立 DNS 服务器
- **SNMP 探测** — 标量 OID `Get` + 表 OID `Walk`，支持索引/标签提取，纯数值 OID（无需 MIB）
- **端口检测** — TCP/UDP 端口连通性检测，输出拨号耗时
- **HTTP 检测** — HTTP 状态码 + 响应体包含字符串检查
- **VictoriaMetrics 推送** — Prometheus 文本格式 via `/api/v1/import/prometheus`，支持批量和指数退避重试
- **MCP 服务** — Streamable HTTP 传输，API Key 鉴权，6 个查询和管理工具（含批量添加），带文件上传端点
- **自身指标** — 代理健康指标暴露在 `/metrics` (Prometheus 抓取端点)
- **热重载** — 通过 `SIGHUP` 信号或 MCP `reload_config` 工具热加载配置
- **目标隔离** — 每个监控目标在独立 goroutine 中运行，拥有独立的超时控制，单目标失败不影响其他目标

---

## 编译

```bash
git clone <repo> && cd ProbeKit

# 构建当前平台
go build -o ProbeKit ./cmd/ProbeKit/

# 注入版本号
go build -ldflags "-X github.com/ZeroYe/probekit/internal/selfmetrics.BuildVersion=v1.0.0" -o ProbeKit ./cmd/ProbeKit/

# 交叉编译
GOOS=linux GOARCH=amd64 go build -o ProbeKit-linux-amd64 ./cmd/ProbeKit/
GOOS=linux GOARCH=arm64 go build -o ProbeKit-linux-arm64 ./cmd/ProbeKit/
GOOS=windows GOARCH=amd64 go build -o ProbeKit-windows-amd64.exe ./cmd/ProbeKit/

# 或使用 Makefile
make build                        # 当前平台
make build VERSION=v1.0.0         # 带版本号
make release                       # 所有平台
```

---

## 配置详解

### 全局配置 `configs/global.yaml`

```yaml
concurrency: 20                    # 最大并发操作数
log_level: info                    # debug / info / warn / error

victoria_metrics:
  push_url: "http://10.0.0.1:8428/api/v1/import/prometheus"
  batch_size: 500                  # 每批最大指标数
  flush_interval: 10s              # 推送间隔
  buffer_size: 10000               # 环形缓冲区大小（超过时丢弃最旧指标）
  retry:
    max_retries: 3                 # 推送失败最大重试次数
    initial_backoff: 1s            # 首次重试等待
    max_backoff: 30s              # 最大重试等待

mcp_server:
  enabled: true
  listen: ":9801"                  # 监听地址，可改为 0.0.0.0:9801 或 127.0.0.1:9801
  auth:
    type: api_key
    keys:
      - "sk-probe-change-me"       # 可配置多个 key 用于轮换

self_metrics:
  enabled: true
  path: /metrics                   # 自身指标 HTTP 路径
  collect_runtime: true            # 是否包含 Go 运行时指标（goroutine 数、内存等）
```

### ICMP 配置 `configs/icmp.yaml`

ICMP 模块有独立的推送通道，可单独设置推送间隔和 batch 大小（省略则继承全局 `victoria_metrics` 默认值）：

```yaml
# 可选：ICMP 模块独立的推送参数
flush_interval: 10s
batch_size: 1000
buffer_size: 20000

histogram_buckets_ms: [1, 5, 10, 20, 50, 100, 200, 500, 1000]  # RTT 直方图桶 (ms)

# 内联目标
targets:
  - host: "8.8.8.8"
    interval: 60s                  # 每 60 秒探测一次
    count: 4                       # 每次发 4 个包
    timeout: 5s                    # 单包超时
    labels:
      region: global
      isp: google

  - host: "114.114.114.114"
    interval: 30s
    count: 2
    timeout: 3s
    labels:
      region: china
      isp: chinanet

# 外部目标文件路径（支持绝对路径或相对路径）
# 文件内容为 targets 数组 YAML，与内联 targets 自动合并
# 适用于 2000+ 大规模目标，可用脚本批量生成
# targets_file: "/etc/probekit/icmp_targets.yaml"
```

**外部目标文件示例** (`/etc/probekit/icmp_targets.yaml`):
```yaml
- host: "10.0.0.1"
  interval: 60s
  count: 4
  timeout: 5s
  labels:
    region: cn-beijing
    rack: A01
- host: "10.0.0.2"
  interval: 60s
  count: 4
  timeout: 5s
  labels:
    region: cn-shanghai
    rack: B03
```

**批量生成脚本** (`scripts/gen-icmp-targets.sh`):
```bash
# 从 IP 列表文件生成（每行一个 IP）
cat ips.txt | ./scripts/gen-icmp-targets.sh --label region=cn-beijing > /etc/probekit/icmp_targets.yaml

# 从 CIDR 段生成
./scripts/gen-icmp-targets.sh --cidr 10.0.0.0/24 --label region=cn-beijing > /etc/probekit/icmp_targets.yaml

# 从 CSV 生成（每行可带独立标签）
./scripts/gen-icmp-targets.sh --csv targets.csv > /etc/probekit/icmp_targets.yaml
```

**ICMP 生成的指标（每个 target）：**

| 指标 | 类型 | 说明 |
|------|------|------|
| `icmp_up` | gauge | 1=成功, 0=全部超时 |
| `icmp_packet_loss_ratio` | gauge | 丢包率 0.0~1.0 |
| `icmp_consecutive_loss` | gauge | 连续丢包数 |
| `icmp_rtt_min_ms` | gauge | 最小 RTT (ms) |
| `icmp_rtt_max_ms` | gauge | 最大 RTT (ms) |
| `icmp_rtt_avg_ms` | gauge | 平均 RTT (ms) |
| `icmp_rtt_stddev_ms` | gauge | RTT 标准差 (ms) |
| `icmp_jitter_ms` | gauge | RTT 抖动（相邻包差绝对值平均） |
| `icmp_rtt_bucket` | gauge | RTT 直方图 (le 标签) |
| `icmp_rtt_count` | gauge | 成功收到回复数 |
| `icmp_rtt_sum` | gauge | RTT 总和 |

### DNS 配置 `configs/dns.yaml`

```yaml
# 可选：模块独立的推送参数（省略则继承全局）
# flush_interval: 10s
# batch_size: 500

targets:
  - domain: "example.com"
    server: "8.8.8.8"             # 指定 DNS 服务器
    record_type: "A"               # A / AAAA / MX / NS / CNAME / TXT
    interval: 60s
    labels:
      type: external

  - domain: "example.com"
    server: "8.8.8.8"
    record_type: "AAAA"            # 同时监控 IPv6 解析
    interval: 60s

  - domain: "mail.example.com"
    server: "1.1.1.1"
    record_type: "MX"
    interval: 120s
    labels:
      service: mail

  - domain: "internal.corp.com"
    server: "10.0.0.53"            # 内网 DNS 服务器
    record_type: "A"
    interval: 30s
```

**DNS 生成的指标（每个 target）：**

| 指标 | 类型 | 说明 |
|------|------|------|
| `dns_up` | gauge | 1=查询成功, 0=失败 |
| `dns_lookup_duration_seconds` | gauge | 查询耗时 (秒) |
| `dns_answer_count` | gauge | 返回的答案数 |
| `dns_response` | gauge | 响应码: 0=NOERROR, 1=NXDOMAIN, 2=TIMEOUT, 3=SERVFAIL |

### SNMP 配置 `configs/snmp.yaml`

```yaml
# 可选：模块独立的推送参数（省略则继承全局）
# flush_interval: 15s
# batch_size: 300

defaults:
  version: "2c"                    # SNMP 版本: 1 / 2c / 3
  community: "public"              # SNMP 团体名
  port: 161                        # 默认端口
  timeout: 5s                      # 连接超时
  retries: 1                       # 重试次数

targets:
  # 示例 1: 路由器 — 基础信息 + 接口流量
  - host: "192.168.1.1"
    interval: 60s
    timeout: 10s                   # 覆盖默认超时
    labels:
      role: core-router
      site: bj
    oids:
      scalar:
        - "1.3.6.1.2.1.1.3.0"     # sysUpTime
        - "1.3.6.1.2.1.1.5.0"     # sysName
        - "1.3.6.1.4.1.9.9.48.1.1.1.6.1"  # cpmCPUTotal5sec (Cisco)
      tables:
        - oid: "1.3.6.1.2.1.2.2"           # ifTable
          index: "ifIndex"
          tag: "ifDescr"                    # 接口描述作为标签
          metrics:
            - oid: "1.3.6.1.2.1.2.2.1.10"  # ifInOctets
              name: "if_in_octets"
              type: "counter"
            - oid: "1.3.6.1.2.1.2.2.1.16"  # ifOutOctets
              name: "if_out_octets"
              type: "counter"
            - oid: "1.3.6.1.2.1.2.2.1.3"   # ifOperStatus
              name: "if_oper_status"
              type: "gauge"

  # 示例 2: 交换机 — 仅标量
  - host: "192.168.1.2"
    interval: 120s
    labels:
      role: access-switch
      site: bj
    oids:
      scalar:
        - "1.3.6.1.2.1.1.3.0"

  # 示例 3: 使用不同团体名的设备
  - host: "10.0.0.1"
    interval: 60s
    labels:
      role: fw
    oids:
      scalar:
        - "1.3.6.1.2.1.1.3.0"
```

**SNMP 生成的指标（每个 target）：**

| 指标 | 类型 | 说明 |
|------|------|------|
| `snmp_up` | gauge | 1=连接成功, 0=失败 |
| `<scalar_oid>` | gauge | 标量 OID 以数值为指标名，值即 OID 返回值 |
| `<table_metric_name>` | gauge/counter | 表指标，含 `index` 和 `tag` 标签 |

> SNMP 标量 OID 的指标名使用纯数值（如 `1_3_6_1_2_1_1_3_0`），表指标使用配置中 `name` 字段。

---

### 端口检测配置 `configs/port.yaml`

```yaml
# 可选：模块独立的推送参数（省略则继承全局）
# flush_interval: 10s
# batch_size: 500

targets:
  - host: "192.168.1.1"
    port: 443
    protocol: tcp               # tcp / udp
    timeout: 5s
    interval: 30s
    labels:
      service: web-https

  - host: "8.8.8.8"
    port: 53
    protocol: udp
    timeout: 5s
    interval: 60s
    labels:
      service: dns
```

**端口检测生成的指标（每个 target）：**

| 指标 | 类型 | 说明 |
|------|------|------|
| `port_up` | gauge | 1=连通, 0=不通 |
| `port_dial_duration_seconds` | gauge | 拨号耗时 (秒) |

### HTTP 检测配置 `configs/http.yaml`

```yaml
# 可选：模块独立的推送参数（省略则继承全局）
# flush_interval: 10s
# batch_size: 500

targets:
  - url: "https://example.com/health"
    method: GET
    expected_status_codes: [200, 301]
    expected_body_contains: "ok"       # 可选，响应体包含此字符串才算 up
    timeout: 10s
    interval: 60s
    headers:
      User-Agent: "ProbeKit/1.0"
    labels:
      service: example

  - url: "https://api.example.com/v1/status"
    method: GET
    expected_status_codes: [200]
    timeout: 5s
    interval: 30s
```

**HTTP 检测生成的指标（每个 target）：**

| 指标 | 类型 | 说明 |
|------|------|------|
| `http_up` | gauge | 1=正常, 0=异常（状态码不匹配或 body 不包含指定字符串） |
| `http_status_code` | gauge | 返回的 HTTP 状态码 |
| `http_duration_seconds` | gauge | 请求耗时 (秒) |
| `http_response_size_bytes` | gauge | 响应体大小 (字节) |

---

## 运行

```bash
# Linux/macOS: ICMP 需要 root 或 CAP_NET_RAW
sudo ./ProbeKit --config-dir ./configs

# 也可以赋予 CAP_NET_RAW 后以普通用户运行
sudo setcap cap_net_raw+ep ./ProbeKit
./ProbeKit --config-dir ./configs

# Windows: 以管理员身份运行
./ProbeKit.exe --config-dir ./configs

# 指定不同的配置文件目录
./ProbeKit --config-dir /etc/ProbeKit
```

### 启动日志示例

```
2026-06-07T04:41:36.790+0800 INFO starting ProbeKit config_dir=./config
2026-06-07T04:41:36.797+0800 INFO vm pusher started push_url=http://victoria:8428/api/v1/import/prometheus  flush_interval=10s  batch_size=500
2026-06-07T04:41:36.797+0800 INFO icmp started targets=2 concurrency=20
2026-06-07T04:41:36.797+0800 INFO dns started targets=3
2026-06-07T04:41:36.797+0800 INFO snmp started targets=2
2026-06-07T04:41:36.797+0800 INFO port started targets=1
2026-06-07T04:41:36.797+0800 INFO http started targets=2
2026-06-07T04:41:36.797+0800 INFO mcp api key auth enabled
2026-06-07T04:41:36.797+0800 INFO self metrics endpoint registered path=/metrics  collect_runtime=true
2026-06-07T04:41:36.798+0800 INFO mcp server starting addr=:9801  endpoint=/mcp
2026-06-07T04:41:36.798+0800 INFO ProbeKit started
```

---

## MCP 工具使用详解

MCP (Model Context Protocol) 允许 AI 助手（如 Claude）直接查询和管理监控系统。连接地址为 `http://host:9801/mcp`。

ProbeKit 提供 **6 个 MCP 工具**：`get_targets`、`get_metrics`、`add_target`、`batch_add_targets`、`remove_target`、`reload_config`。另外 `add_target` 和 `remove_target` 也支持 Port / HTTP 模块。

### 准备

```bash
# 确保 global.yaml 中已配置 API Key
# mcp_server.auth.keys: ["sk-probe-change-me"]
```

### 测试连接

```bash
# MCP 端点需要 POST 请求 + JSON body
curl -s -X POST http://localhost:9801/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-probe-change-me" \
  -d '{}'
```

### 外部目标文件与 MCP 工具的关系

当模块使用 `targets_file`（如 icmp.yaml 中的 `targets_file: /etc/probekit/icmp_targets.yaml`）时：

- **`get_targets`**：可以正常查询外部文件的全部目标（配置加载时会合并到内存）
- **`add_target` / `remove_target`**：操作的是 icmp.yaml 中的内联 `targets` 列表，而非外部文件
- **`batch_add_targets`**：支持追加到外部文件——当模块配置了 `targets_file` 时，新增目标会追加到该文件

对于外部文件中的大量目标，建议先用脚本批量生成，再通过 AI 辅助的 `batch_add_targets` 增量管理。

### 1. `get_targets` — 查询所有监控目标

列出所有目标、所属模块和 UP/DOWN 状态。支持所有 6 个模块（icmp / dns / snmp / port / http）。

```bash
# 查询全部目标
curl -s -X POST http://localhost:9801/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-probe-change-me" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "get_targets",
      "arguments": {}
    }
  }' | jq .

# 返回示例
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "Found 7 target(s):\n\n[UP] icmp [8.8.8.8] labels=map[region:global isp:google]\n[UP] icmp [114.114.114.114] labels=map[region:china isp:chinanet]\n[UP] dns [example.com] labels=map[record_type:A server:8.8.8.8]\n[UP] dns [mail.example.com] labels=map[record_type:MX server:1.1.1.1]\n[UP] snmp [192.168.1.1] labels=map[role:core-router]\n[UP] snmp [192.168.1.2] labels=map[role:access-switch]\n[DOWN] snmp [10.0.0.1] labels=map[role:fw]"
      }
    ],
    "isError": false
  }
}
```

```bash
# 按模块查询
curl -s -X POST http://localhost:9801/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-probe-change-me" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "get_targets",
      "arguments": {
        "module": "icmp"
      }
    }
  }' | jq .
```

**AI 助手用法示例：**
> 用户："检查 ProbeKit 所有目标的状态"
> AI 调用 `get_targets` → 返回 7 个目标，其中 snmp 10.0.0.1 DOWN → AI 报告异常

### 2. `get_metrics` — 查询目标的最新指标

```bash
# 查询 ICMP 目标 8.8.8.8 的指标
curl -s -X POST http://localhost:9801/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-probe-change-me" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "get_metrics",
      "arguments": {
        "target": "8.8.8.8"
      }
    }
  }' | jq .

# 返回示例（节选）
{
  "result": {
    "content": [
      {
        "type": "text",
        "text": "Metrics for 8.8.8.8:\n\n--- icmp/8.8.8.8 ---\n  icmp_up = 1.000\n  icmp_packet_loss_ratio = 0.000\n  icmp_rtt_avg_ms = 12.345\n  icmp_rtt_max_ms = 15.678\n  icmp_rtt_min_ms = 10.123\n  icmp_jitter_ms = 1.234\n  icmp_rtt_bucket{le=\"10\"} = 1\n  icmp_rtt_bucket{le=\"20\"} = 4\n  icmp_rtt_bucket{le=\"+Inf\"} = 4\n  icmp_rtt_count = 4\n  icmp_rtt_sum = 49.380"
      }
    ]
  }
}
```

```bash
# 指定模块（加速查询）
curl -s -X POST http://localhost:9801/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-probe-change-me" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "get_metrics",
      "arguments": {
        "target": "192.168.1.1",
        "module": "snmp"
      }
    }
  }' | jq .
```

**AI 助手用法示例：**
> 用户："看看 8.8.8.8 的延迟情况"
> AI 调用 `get_metrics("8.8.8.8")` → 返回 avg=12.3ms, max=15.6ms, 丢包率=0 → AI 报告"延迟正常，无丢包"

### 3. `add_target` — 添加监控目标

```bash
# 添加 ICMP 目标
curl -s -X POST http://localhost:9801/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-probe-change-me" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "add_target",
      "arguments": {
        "module": "icmp",
        "host": "1.1.1.1",
        "interval": "30s",
        "labels": "{\"isp\":\"cloudflare\",\"region\":\"global\"}"
      }
    }
  }' | jq .

# 返回
{
  "result": {
    "content": [
      {
        "type": "text",
        "text": "added target 1.1.1.1 to icmp and reloaded config"
      }
    ]
  }
}
```

```bash
# 添加 DNS 目标
curl -s -X POST http://localhost:9801/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-probe-change-me" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "add_target",
      "arguments": {
        "module": "dns",
        "host": "github.com",
        "server": "8.8.8.8",
        "record_type": "A",
        "interval": "60s"
      }
    }
  }' | jq .
```

```bash
# 添加 SNMP 目标
curl -s -X POST http://localhost:9801/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-probe-change-me" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "add_target",
      "arguments": {
        "module": "snmp",
        "host": "10.0.0.2",
        "interval": "60s",
        "labels": "{\"role\":\"server\"}"
      }
    }
  }' | jq .
```

```bash
# 添加 Port 目标
curl -s -X POST http://localhost:9801/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-probe-change-me" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "add_target",
      "arguments": {
        "module": "port",
        "host": "192.168.1.1",
        "port": 443,
        "protocol": "tcp",
        "interval": "30s"
      }
    }
  }' | jq .
```

```bash
# 添加 HTTP 目标
curl -s -X POST http://localhost:9801/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-probe-change-me" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "add_target",
      "arguments": {
        "module": "http",
        "host": "https://example.com/health",
        "method": "GET",
        "interval": "60s",
        "labels": "{\"service\":\"example\"}"
      }
    }
  }' | jq .
```

**AI 助手用法示例：**
> 用户："帮我添加一个新的监控目标，ICMP ping 1.1.1.1"
> AI 调用 `add_target("icmp", "1.1.1.1", "30s")` → 配置写入 YAML，采集器热重启 → AI 确认"已添加 1.1.1.1，30 秒间隔"

### 4. `batch_add_targets` — 批量添加监控目标

一次性添加多个目标。支持**两种模式**：

#### 模式 A：命令行参数模式（`hosts` + 公共参数）

适用于多个主机共用相同参数的场景：

```bash
# 从 IP 列表批量添加 ICMP 目标
curl -s -X POST http://localhost:9801/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-probe-change-me" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "batch_add_targets",
      "arguments": {
        "module": "icmp",
        "hosts": "10.0.0.1,10.0.0.2,10.0.0.3",
        "interval": "60s",
        "labels": "{\"region\":\"cn-beijing\",\"role\":\"gateway\"}"
      }
    }
  }' | jq .

# 返回示例
{
  "result": {
    "content": [
      {
        "type": "text",
        "text": "added 3 targets to icmp and reloaded config"
      }
    ]
  }
}
```

```bash
# 从 CIDR 段批量添加（展开所有 IP）
curl -s -X POST http://localhost:9801/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-probe-change-me" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "batch_add_targets",
      "arguments": {
        "module": "icmp",
        "hosts": "10.0.0.0/28",
        "interval": "60s",
        "count": 4,
        "timeout": "5s",
        "labels": "{\"region\":\"cn-beijing\"}"
      }
    }
  }' | jq .
```

**参数说明（模式 A）：**
- `module` — 模块名（icmp / dns / snmp / port / http）
- `hosts` — 逗号分隔的 IP/域名列表，或 CIDR 段（如 `10.0.0.0/24`）
- `interval` — 探测间隔，默认 60s
- `count` — 仅 ICMP 模块，发包数，默认 4
- `timeout` — 超时时间，默认 5s
- `port` — 仅 Port 模块，端口号
- `protocol` — 仅 Port 模块，`tcp` 或 `udp`
- `method` / `expected_body_contains` — 仅 HTTP 模块
- `labels` — JSON 格式标签，如 `{"region":"cn-beijing"}`

**AI 助手用法示例：**
> 用户："帮我监控 10.0.0.1 到 10.0.0.10 这些 IP"
> AI 调用 `batch_add_targets("icmp", "10.0.0.1,10.0.0.2,...,10.0.0.10", "60s")` → 10 个目标写入 YAML → AI 确认"已添加 10 个 ICMP 目标"

#### 模式 B：CSV 模式（`csv` / `csv_file` 参数）

适用于每个目标有不同参数（如不同标签、不同端口）的场景。根据数据量选择方式：

- **小批量（< 100 行）**：用 `csv` 参数，AI 助手先输出 CSV 模板，用户填写后发回
- **大批量（100~10000 行）**：用 `csv_file` 参数，在 ProbeKit 服务器上准备好 CSV 文件，AI 直接读取

**工作流程（小批量）：**

```
用户: "帮我批量添加这些 IP 到 ICMP 监控"
  → AI 生成 CSV 模板发给用户
  → 用户填写 CSV 发回给 AI
  → AI 调用 batch_add_targets(module="icmp", csv="...")
  → 目标写入 YAML，配置重载
```

**工作流程（大批量，推荐）：**

#### 方式一：服务器本地已有 CSV 文件

```bash
# 在服务器上准备好 CSV 文件
cat > /tmp/targets.csv << 'CSV'
host,interval,count,timeout,labels
10.0.0.1,60s,4,5s,region=cn-beijing|rack=A01
10.0.0.2,60s,4,5s,region=cn-shanghai|rack=B03
CSV
```

```
用户: "我 /tmp/targets.csv 里准备了 1000 个目标，帮我加到 ICMP 监控"
  → AI 调用 batch_add_targets(module="icmp", csv_file="/tmp/targets.csv")
  → 读取本地文件、解析 CSV、追加到 targets_file、重载配置
  → "已添加 1000 个 ICMP 目标"
```

#### 方式二：通过 HTTP 上传文件（从本机上传）

ProbeKit 的 MCP 服务提供 `POST /upload` 上传端点，用户可直接从本机上传 CSV 文件到服务器：

```bash
# 使用 API Key 认证上传文件
curl -X POST http://localhost:9801/upload \
  -H "Authorization: Bearer sk-probe-change-me" \
  -F "file=@/path/to/local/targets.csv"

# 返回上传后的文件路径，例如：
# /etc/probekit/uploads/a1b2c3d4_targets.csv
```

上传后返回完整路径，用户告诉 AI 路径，AI 调用 `batch_add_targets` 处理：

```
用户: "我上传了 CSV 到服务器，路径是 /etc/probekit/uploads/a1b2c3d4_targets.csv"
  → AI 调用 batch_add_targets(module="icmp", csv_file="/etc/probekit/uploads/a1b2c3d4_targets.csv")
  → 读取、解析、追加、重载
  → "已添加 1000 个 ICMP 目标"
```

**各模块 CSV 模板示例：**

ICMP:
```csv
host,interval,count,timeout,labels
10.0.0.1,60s,4,5s,region=cn-beijing|rack=A01|role=gw
10.0.0.2,60s,4,5s,region=cn-shanghai|rack=B03|role=sw
10.0.0.3,30s,4,5s,region=cn-beijing|rack=A02|role=web
```

DNS:
```csv
host,server,record_type,interval,labels
example.com,8.8.8.8,A,60s,type=external
internal.corp.com,10.0.0.53,A,30s,type=internal
mail.example.com,1.1.1.1,MX,120s,service=mail
```

Port:
```csv
host,port,protocol,timeout,interval,labels
192.168.1.1,443,tcp,5s,30s,service=web-https
8.8.8.8,53,udp,5s,60s,service=dns
```

HTTP:
```csv
host,method,interval,timeout,labels
https://example.com/health,GET,60s,10s,service=example
https://api.example.com/status,GET,30s,5s,service=api
```

**支持的 CSV 列名（大小写不敏感）：**

| 列名 | 说明 | 适用模块 |
|------|------|----------|
| `host` / `ip` / `domain` / `url` | 主机/IP/域名/URL | 全部 |
| `interval` | 探测间隔 | 全部 |
| `timeout` | 超时 | icmp, port, http |
| `count` | 发包数 | icmp |
| `port` | 端口号 | port |
| `protocol` | 协议 tcp/udp | port |
| `server` | DNS 服务器 | dns |
| `record_type` / `type` | DNS 记录类型 | dns |
| `method` | HTTP 方法 | http |
| `labels` / `tags` | 标签，`key=value\|key2=value2` 格式 | 全部 |

**AI 助手用法示例（CSV 模式）：**
> **用户：** "帮我把这些机器加到 ICMP 监控，每台机器的标签不一样"
>
> **AI：** 好的，这是 CSV 模板，请填写每台机器的信息：
> ```csv
> host,interval,count,timeout,labels
> ,60s,4,5s,
> ,60s,4,5s,
> ```
> 
> **用户：** "填好了：
> ```csv
> host,interval,count,timeout,labels
> 10.0.0.1,60s,4,5s,region=cn-beijing|rack=A01
> 10.0.0.2,60s,4,5s,region=cn-shanghai|rack=B03
> 10.0.0.3,30s,4,5s,region=cn-beijing|rack=A02
> ```"
>
> **AI 调用 `batch_add_targets(module="icmp", csv="...")` → 确认"已添加 3 个 ICMP 目标"**

### 5. `remove_target` — 删除监控目标

```bash
# 删除 ICMP 目标
curl -s -X POST http://localhost:9801/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-probe-change-me" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "remove_target",
      "arguments": {
        "module": "icmp",
        "host": "1.1.1.1"
      }
    }
  }' | jq .

# 返回
{
  "result": {
    "content": [
      {
        "type": "text",
        "text": "removed target 1.1.1.1 from icmp and reloaded config"
      }
    ]
  }
}
```

```bash
# 目标不存在时
curl -s -X POST ... -d '{"name":"remove_target","arguments":{"module":"icmp","host":"9.9.9.9"}}' | jq .
# 返回: "target 9.9.9.9 not found in icmp"
```

**AI 助手用法示例：**
> 用户："把 1.1.1.1 从监控中移除"
> AI 调用 `remove_target("icmp", "1.1.1.1")` → 从 YAML 删除，热重启 → AI 确认"已移除"

### 6. `reload_config` — 热重载配置

```bash
# 手动编辑了 configs/ 目录下的 YAML 文件后，触发重载
curl -s -X POST http://localhost:9801/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-probe-change-me" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "reload_config",
      "arguments": {}
    }
  }' | jq .

# 返回
{
  "result": {
    "content": [
      {
        "type": "text",
        "text": "config reloaded successfully"
      }
    ]
  }
}
```

**AI 助手用法示例：**
> 用户："我刚编辑了 dns.yaml 加了几个域名，帮我重载配置"
> AI 调用 `reload_config()` → 日志显示 config reloaded → AI 确认"已重载，新目标已生效"

---

## Prometheus / VictoriaMetrics 抓取配置

### VictoriaMetrics vmagent 或 vmalert

```yaml
scrape_configs:
  - job_name: ProbeKit
    scrape_interval: 15s
    static_configs:
      - targets: ["localhost:9801"]
    metrics_path: /metrics
```

### Categraf

```ini
[[inputs.prometheus]]
  url = "http://localhost:9801/metrics"
  interval = 15
```

### 自身指标 Prometheus 输出示例

```
# HELP probe_agent_info
# TYPE probe_agent_info gauge
probe_agent_info{goversion="go1.26.4",version="dev"} 1
# HELP probe_agent_uptime_seconds
# TYPE probe_agent_uptime_seconds gauge
probe_agent_uptime_seconds 3600
# HELP probe_agent_metrics_collected_total
# TYPE probe_agent_metrics_collected_total counter
probe_agent_metrics_collected_total 5420
# HELP probe_agent_metrics_pushed_total
# TYPE probe_agent_metrics_pushed_total counter
probe_agent_metrics_pushed_total 5390
# HELP probe_agent_push_errors_total
# TYPE probe_agent_push_errors_total counter
probe_agent_push_errors_total 2
# HELP probe_agent_queue_length
# TYPE probe_agent_queue_length gauge
probe_agent_queue_length 0
# HELP probe_agent_targets
# TYPE probe_agent_targets gauge
probe_agent_targets{module="icmp"} 2
probe_agent_targets{module="dns"} 3
probe_agent_targets{module="snmp"} 2
probe_agent_targets{module="port"} 1
probe_agent_targets{module="http"} 2
# HELP probe_agent_goroutines
# TYPE probe_agent_goroutines gauge
probe_agent_goroutines 15
# HELP probe_agent_memory_alloc_bytes
# TYPE probe_agent_memory_alloc_bytes gauge
probe_agent_memory_alloc_bytes 4194304
# HELP probe_agent_memory_heap_bytes
# TYPE probe_agent_memory_heap_bytes gauge
probe_agent_memory_heap_bytes 4194304
# HELP probe_agent_memory_sys_bytes
# TYPE probe_agent_memory_sys_bytes gauge
probe_agent_memory_sys_bytes 16777216
# HELP probe_agent_gc_pauses_total
# TYPE probe_agent_gc_pauses_total counter
probe_agent_gc_pauses_total 3
```

---

## Grafana 面板思路

查询 ProbeKit 自身指标的 PromQL 示例：

| 需求 | PromQL |
|------|--------|
| 采集器是否存活 | `probe_agent_uptime_seconds` |
| 各模块目标数 | `probe_agent_targets` |
| 推送错误率 | `rate(probe_agent_push_errors_total[5m])` |
| 推送积压 | `probe_agent_queue_length` |
| 内存使用 | `probe_agent_memory_alloc_bytes` |
| Goroutine 数 | `probe_agent_goroutines` |

对于采集到的监控指标（需从 VictoriaMetrics 查询）：

| 需求 | PromQL |
|------|--------|
| ICMP 延迟 P99 | `histogram_quantile(0.99, sum(rate(icmp_rtt_bucket[5m])) by (le))` |
| ICMP 丢包率 | `avg(icmp_packet_loss_ratio)` |
| DNS 解析耗时 | `avg(dns_lookup_duration_seconds)` |
| DNS 失败率 | `avg(1 - dns_up)` |
| SNMP 可达性 | `avg(snmp_up)` |
| 接口流量 | `rate(if_in_octets[5m]) * 8` |
| 端口连通率 | `avg(port_up)` |
| HTTP 可用率 | `avg(http_up)` |
| HTTP 响应耗时 | `avg(http_duration_seconds)` |

---

## 常见问题

### 问：为什么 SNMP 不支持 MIB 名称？

答：纯数值 OID 无需 MIB 编译，跨平台兼容，Windows/Linux/macOS 行为一致，且方便从其他工具复用 OID 配置。

### 问：能支持多少监控目标？

答：ICMP 使用持久化 socket + 信号量并发控制（默认 20），可支持 **2000+ 目标**。DNS/SNMP/Port/HTTP 目标数受限于 goroutine 开销和网络带宽，单机通常可运行数千目标。每个目标独立 goroutine 运行，通过 channel 解耦采集与推送。

### 问：VictoriaMetrics 宕机怎么办？

答：指标暂存在环形缓冲区中（`buffer_size` 可配置，默认 10000），推送器以指数退避策略重试（`max_retries`/`initial_backoff`/`max_backoff` 可配置）。缓冲区满时丢弃最旧指标（FIFO），不影响采集器继续运行。

### 问：可以不用 VictoriaMetrics 吗？

答：可以。将 global.yaml 中 `push_url` 留空即可禁用推送；指标仍保留在内存 registry 中，可通过 MCP `get_metrics` 工具实时查询。

### 问：如何更新配置而不重启进程？

答：两种方式：
1. 发送 `SIGHUP` 信号：`kill -HUP <pid>`
2. 通过 MCP 调用 `reload_config` 工具（见上）
两者都会重读所有 YAML 文件、停止旧采集器、启动新采集器。

### 问：MCP 鉴权失败怎么办？

```bash
# 401 响应
curl -s -X POST http://localhost:9801/mcp -H "Content-Type: application/json" -d '{}'
# 返回: 401 Unauthorized

# 确保请求头中包含正确的 Authorization
curl -s -X POST http://localhost:9801/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-probe-change-me" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_targets","arguments":{}}}'
```

### 问：Windows 上运行有什么注意事项？

- ICMP 需要以管理员身份运行（否则原始套接字创建失败）
- 如果不需要 ICMP，可在 icmp.yaml 中留空 targets 列表，或删除 icmp.yaml 文件
- 路径使用 `\` 或 `/` 均可，建议用 `--config-dir ./configs`

### 问：如何配置日志轮转？

当前日志输出到 stderr，建议使用系统工具管理：
- Linux: `systemd-journald` 或 `logrotate` 配合重定向
- Windows: 重定向到文件 `.\ProbeKit.exe --config-dir .\configs > agent.log 2>&1`，配合第三方轮转工具

---

## 信号

| 信号 | 行为 |
|------|------|
| `SIGINT` / `SIGTERM` | 优雅关闭（刷新缓冲中的指标后退出） |
| `SIGHUP` | 配置热重载（重读 YAML，重启采集器） |

## Linux 权限 (ICMP)

ICMP 原始套接字需要 `CAP_NET_RAW` 权限。三种方式：

```bash
# 方式 1: root 运行
sudo ./ProbeKit --config-dir ./configs

# 方式 2: 赋予 capability（推荐）
sudo setcap cap_net_raw+ep ./ProbeKit
./ProbeKit --config-dir ./configs

# 方式 3: 不加 ICMP 目标，只使用 DNS/SNMP
# 在 icmp.yaml 中留空 targets: []
```

---

## 版本信息

在构建时通过 ldflags 注入版本号：

```bash
go build -ldflags "-X github.com/ZeroYe/probekit/internal/selfmetrics.BuildVersion=v1.2.3" -o ProbeKit ./cmd/ProbeKit/
```

版本号会在 `/metrics` 端点的 `probe_agent_info{version="v1.2.3"}` 指标和 MCP 服务初始化日志中体现。
