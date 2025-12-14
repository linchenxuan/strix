# Prometheus 部署指南

本指南提供 Docker Compose 部署 Prometheus 的完整方案。

## 快速开始

### Docker 部署

```bash
# 使用部署脚本
cd deployments
./scripts/deploy-prometheus.sh docker

# 或者手动部署
docker-compose up -d
```

## 服务访问

- **Prometheus**: http://localhost:9090
- **Grafana**: http://localhost:3000 (默认账号: admin/admin)
- **应用服务**: http://localhost:8080
- **Node Exporter**: http://localhost:9100

## 配置文件说明

### Prometheus 配置 (`prometheus.yml`)
- **scrape_interval**: 抓取间隔时间（默认 15s）
- **evaluation_interval**: 规则评估间隔时间
- **scrape_configs**: 定义监控目标和抓取规则

### Docker Compose 配置 (`docker-compose.yml`)
- **prometheus**: Prometheus 主服务
- **grafana**: 可视化界面（可选）
- **node-exporter**: 主机指标收集（可选）
- **app**: 您的应用服务

## 指标集成

### 应用指标暴露

确保您的应用在 `/metrics` 路径暴露 Prometheus 指标：

```go
import (
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "net/http"
)

func init() {
    http.Handle("/metrics", promhttp.Handler())
    go http.ListenAndServe(":8080", nil)
}
```

### 自定义指标

使用项目中的 metrics 包创建自定义指标：

```go
import "github.com/lcx/asura/metrics"

// 创建计数器
metrics.UpdateCounter("requests_total", 1)

// 创建仪表盘
metrics.UpdateGauge("active_connections", 5)

// 记录耗时
stopwatch := metrics.RecordStopwatch("operation_duration")
// ... 执行操作
stopwatch.Stop()
```

## 监控告警

### 创建告警规则

编辑 `prometheus.yml` 添加告警规则：

```yaml
rule_files:
  - "alerts.yml"
```

创建 `alerts.yml`：

```yaml
groups:
- name: asura-alerts
  rules:
  - alert: HighErrorRate
    expr: rate(requests_errors_total[5m]) > 0.1
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "高错误率告警"
      description: "错误率超过 10%"
```

## 清理资源

```bash
# Docker 清理
docker-compose down -v

# 或者使用脚本
./scripts/deploy-prometheus.sh cleanup
```

## 故障排查

### Docker 部署问题

```bash
# 查看服务日志
docker-compose logs prometheus
docker-compose logs app

# 检查服务状态
docker-compose ps

# 重启服务
docker-compose restart prometheus
```

## 性能优化

### Prometheus 调优
- 调整 `storage.tsdb.retention.time` 控制数据保留时间
- 增加内存限制以处理更多指标
- 使用远程存储扩展存储能力

### 应用优化
- 合理设置指标标签，避免高基数
- 使用直方图而非摘要统计耗时
- 定期清理不再使用的指标

## 相关链接

- [Prometheus 官方文档](https://prometheus.io/docs/)
- [Grafana 官方文档](https://grafana.com/docs/)
- [Docker 官方文档](https://docs.docker.com/)
- [Kubernetes 官方文档](https://kubernetes.io/docs/)