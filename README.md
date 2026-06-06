# ProbeKit

一个轻量级网络监控代理，支持 ICMP Ping、DNS 查询和 SNMP 轮询，输出到 VictoriaMetrics，并提供 MCP (Model Context Protocol) 服务以支持 AI 辅助运维。

## 功能特性

- **ICMP 探测** — 多包 Ping，含 RTT 直方图 (p50/p90/p99)、抖动、标准差、丢包率、连续丢包计数
- **DNS 探测** — A/AAAA/MX/NS/CNAME/TXT 查询，每个目标可指定独立 DNS 服务器
- **SNMP 探测** — 标量 OID `Get` + 表 OID `Walk`，支持索引/标签提取，纯数值 OID（无需 MIB）
- **VictoriaMetrics 推送** — Prometheus 文本格式 via `/api/v1/import/prometheus`，支持批量和指数退避重试
- **MCP 服务** — Streamable HTTP 传输，API Key 鉴权，5 个查询和管理工具
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

### 全局配置 `config/global.yaml`

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

### ICMP 配置 `config/icmp.yaml`

```yaml
histogram_buckets_ms: [1, 5, 10, 20, 50, 100, 200, 500, 1000]  # RTT 直方图桶 (ms)

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

### DNS 配置 `config/dns.yaml`

```yaml
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

### SNMP 配置 `config/snmp.yaml`

```yaml
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

## 运行

```bash
# Linux/macOS: ICMP 需要 root 或 CAP_NET_RAW
sudo ./ProbeKit --config-dir ./config

# 也可以赋予 CAP_NET_RAW 后以普通用户运行
sudo setcap cap_net_raw+ep ./ProbeKit
./ProbeKit --config-dir ./config

# Windows: 以管理员身份运行
./ProbeKit.exe --config-dir ./config

# 指定不同的配置文件目录
./ProbeKit --config-dir /etc/ProbeKit
```

### 启动日志示例

```
2026-06-07T04:41:36.790+0800 INFO starting ProbeKit config_dir=./config
2026-06-07T04:41:36.797+0800 INFO vm pusher started push_url=http://victoria:8428/api/v1/import/prometheus  flush_interval=10s  batch_size=500
2026-06-07T04:41:36.797+0800 INFO icmp started targets=2
2026-06-07T04:41:36.797+0800 INFO dns started targets=3
2026-06-07T04:41:36.797+0800 INFO snmp started targets=2
2026-06-07T04:41:36.797+0800 INFO mcp api key auth enabled
2026-06-07T04:41:36.797+0800 INFO self metrics endpoint registered path=/metrics  collect_runtime=true
2026-06-07T04:41:36.798+0800 INFO mcp server starting addr=:9801  endpoint=/mcp
2026-06-07T04:41:36.798+0800 INFO ProbeKit started
```

---

## MCP 工具使用详解

MCP (Model Context Protocol) 允许 AI 助手（如 Claude）直接查询和管理监控系统。连接地址为 `http://host:9801/mcp`。

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

### 1. `get_targets` — 查询所有监控目标

列出所有目标、所属模块和 UP/DOWN 状态。

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

**AI 助手用法示例：**
> 用户："帮我添加一个新的监控目标，ICMP ping 1.1.1.1"
> AI 调用 `add_target("icmp", "1.1.1.1", "30s")` → 配置写入 YAML，采集器热重启 → AI 确认"已添加 1.1.1.1，30 秒间隔"

### 4. `remove_target` — 删除监控目标

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

### 5. `reload_config` — 热重载配置

```bash
# 手动编辑了 config/ 目录下的 YAML 文件后，触发重载
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

---

## 常见问题

### 问：为什么 SNMP 不支持 MIB 名称？

答：纯数值 OID 无需 MIB 编译，跨平台兼容，Windows/Linux/macOS 行为一致，且方便从其他工具复用 OID 配置。

### 问：能支持多少监控目标？

答：设计目标为 50–500 个 SNMP 目标 + 50–100 个 ICMP 目标 + 20–50 个 DNS 目标。每个目标独立 goroutine 运行，通过 channel 解耦采集与推送。

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
- 路径使用 `\` 或 `/` 均可，建议用 `--config-dir ./config`

### 问：如何配置日志轮转？

当前日志输出到 stderr，建议使用系统工具管理：
- Linux: `systemd-journald` 或 `logrotate` 配合重定向
- Windows: 重定向到文件 `.\ProbeKit.exe --config-dir .\config > agent.log 2>&1`，配合第三方轮转工具

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
sudo ./ProbeKit --config-dir ./config

# 方式 2: 赋予 capability（推荐）
sudo setcap cap_net_raw+ep ./ProbeKit
./ProbeKit --config-dir ./config

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
