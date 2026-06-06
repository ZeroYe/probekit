# 开发指南

## 技术栈

| 层 | 选择 |
|---|---|
| 语言 | Go 1.26.4 |
| 模块 | `ProbeKit` |
| 日志 | `go.uber.org/zap` |
| 配置 | `gopkg.in/yaml.v3` |
| ICMP 探测 | `golang.org/x/net/icmp` |
| DNS 探测 | `net` 标准库 |
| SNMP 探测 | `github.com/gosnmp/gosnmp` |
| MCP 服务 | `github.com/mark3labs/mcp-go` |
| 指标输出 | Prometheus 文本格式 → VictoriaMetrics HTTP API |

## 项目结构

```
ProbeKit/
├── cmd/ProbeKit/main.go    # 程序入口
├── config/                    # 示例 YAML 配置文件
├── internal/
│   ├── collector/             # 采集器实现 (ICMP, DNS, SNMP)
│   │   ├── collector.go       #   Collector 接口 + Manager
│   │   ├── icmp.go
│   │   ├── dns.go
│   │   └── snmp.go
│   ├── config/                # 配置结构体 + YAML 加载
│   │   ├── config.go          #   顶层 Config, Load()
│   │   ├── global.go          #   GlobalConfig, VMConfig, MCPConfig
│   │   ├── icmp.go            #   ICMPConfig, ICMPTarget
│   │   ├── dns.go
│   │   └── snmp.go
│   ├── logger/                # zap 日志封装
│   ├── mcp/                   # MCP HTTP 服务 + 工具
│   │   ├── server.go          #   Server (ServeMux, 生命周期)
│   │   ├── auth.go            #   API Key 中间件
│   │   ├── tools_query.go     #   get_targets, get_metrics
│   │   └── tools_manage.go    #   add_target, remove_target, reload_config
│   ├── metrics/               # 核心数据类型
│   │   ├── metric.go          #   Metric 结构体, MetricPool
│   │   ├── histogram.go       #   Prometheus 风格 Histogram
│   │   └── registry.go        #   线程安全的键值指标存储
│   ├── output/                # 管道 → VictoriaMetrics
│   │   ├── pipeline.go        #   Pipeline 编排器
│   │   ├── vmpush.go          #   HTTP 推送 (含重试)
│   │   ├── batch.go           #   Prometheus 文本格式化
│   │   └── buffer.go          #   RingBuffer
│   └── selfmetrics/           # 代理自身监控
│       ├── self.go            #   原子计数器, 构建信息
│       ├── collector.go       #   Collect() → 16 个指标
│       └── handler.go         #   /metrics HTTP 处理器
```

## 构建

```bash
# 构建当前平台
make build

# 交叉编译所有目标
make release

# 注入版本号
make build VERSION=v1.0.0

# 或手动指定
go build -ldflags "-X github.com/ZeroYe/probekit/internal/selfmetrics.BuildVersion=v1.0.0" \
  -o ProbeKit ./cmd/ProbeKit/

# 交叉编译单个目标
GOOS=linux GOARCH=arm64 go build -o ProbeKit-linux-arm64 ./cmd/ProbeKit/
```

### 支持平台

| 目标 | 构建命令 |
|---|---|
| Linux amd64 | `make build-linux-amd64` |
| Linux arm64 | `make build-linux-arm64` |
| Windows amd64 | `make build-windows-amd64` |


## 测试

```bash
# 全部测试
make test

# 带竞态检测
make test-race

# 详细输出
make test-verbose

# 单个包
go test -v -count=1 ./internal/metrics/
```

## 运行

```bash
# 源码运行
go run ./cmd/ProbeKit/ --config-dir ./config

# 编译后运行
./ProbeKit --config-dir ./config
```

## 架构

```
采集器 (ICMP/DNS/SNMP)
    │  每个 target 独立 goroutine, 定时执行
    ▼
Pipeline.Submit(key, metrics)
    │
    ├─► Registry.Store(key, metrics)   ← 最新快照, 供 MCP 查询
    │
    └─► input chan ─► VMPusher
                        │
                        ├─► RingBuffer     ← 解耦采集与推送
                        ├─► Batcher        ← Prometheus 文本格式化
                        └─► HTTP POST      ← VictoriaMetrics /api/v1/import/prometheus

MCP 服务 (:9801)
    ├─► /mcp     → StreamableHTTPServer (可选鉴权)
    ├─► /metrics → 自身指标处理器 (无鉴权)
    └─► 5 个工具 (get_targets, get_metrics, add_target, remove_target, reload_config)
```

## 新增采集器

1. 创建 `internal/collector/<name>.go` 实现 `Collector` 接口：

```go
type Collector interface {
    Name() string
    Start(ctx context.Context, pipeline *output.Pipeline) error
    Stop() error
}
```

2. 在 `internal/config/<name>.go` 中创建对应配置类型，实现 `Validate()`。
3. 在 `internal/config/config.go` 中添加配置结构体字段。
4. 在 `cmd/ProbeKit/main.go` 中通过 `colMgr.Add(collector.NewXxxCollector(...))` 注册。
5. 添加配置文件 `config/<name>.yaml`。
6. 在 `restartCollectors()` 和 MCP 管理工具中添加对应逻辑。

### 采集器职责

- **Start()**: 为每个 target 启动一个 goroutine；每个 goroutine 在定时器上运行，调用探测函数，构造 `[]metrics.Metric`，调用 `pipeline.Submit(key, ms)`。
- **Stop()**: 通过 context 取消或 channel 关闭来通知 goroutine 退出。
- **指标 key**: 使用 `"<module>/<host>"` 格式（如 `"icmp/8.8.8.8"`）。`get_metrics` 工具和 registry 使用这个 key。
- **标签**: 在每个指标上包含 target 特有标签（host 等），以便 Prometheus 过滤。

## MCP Tool 开发

每个工具由注册 + 处理器两部分组成：

```go
func (s *Server) registerMyTool() {
    tool := mcpcore.NewTool("my_tool",
        mcpcore.WithString("param", mcpcore.Required(), mcpcore.Description("参数说明")),
    )
    s.mcpServer.AddTool(tool, s.handleMyTool)
}

func (s *Server) handleMyTool(ctx context.Context, req mcpcore.CallToolRequest) (*mcpcore.CallToolResult, error) {
    param := argString(req, "param")
    return mcpcore.NewToolResultText("结果"), nil
}
```

在 `registerTools()` 中添加 `s.registerMyTool()` 调用（位于 `server.go`）。

## 自身指标

自身指标由 `internal/selfmetrics/collector.go` 在每次 HTTP scrape 时生成。计数器在热路径（Pipeline.Submit → MetricsCollected, VMPusher.pushOnce → MetricsPushed/PushErrors）中使用 `atomic.Int64` 无锁更新。

## 版本号注入

```bash
go build -ldflags "-X github.com/ZeroYe/probekit/internal/selfmetrics.BuildVersion=v1.2.3" -o ProbeKit ./cmd/ProbeKit/
```

版本号会在 `/metrics` 端点的 `probe_agent_info{version="v1.2.3"}` 指标中体现。

## 配置热重载

通过 `SIGHUP` 信号或 MCP `reload_config` 工具触发：
1. 重新读取 config 目录下的所有 YAML 文件
2. 停止现有采集器
3. 使用新配置创建新采集器
4. 启动新采集器
5. 更新 MCP API Key
6. Config 指针原地更新
