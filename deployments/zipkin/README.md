# Zipkin 部署说明

此目录包含用于在本地部署 Zipkin 服务的 `docker-compose` 配置。

## 启动 Zipkin

1.  确保您已安装 Docker 和 Docker Compose。
2.  导航到此目录：
    ```bash
    cd strix/deployments/zipkin
    ```
3.  运行以下命令启动 Zipkin 服务：
    ```bash
    docker-compose up -d
    ```
    `-d` 标志表示在后台运行服务。

## 访问 Zipkin UI

Zipkin UI 应该在您的浏览器中通过以下地址可用：

[http://localhost:9411](http://localhost:9411)

## 停止 Zipkin

要停止并移除 Zipkin 服务，请在此目录中运行：

```bash
docker-compose down
```
这将停止并移除由 `docker-compose.yml` 文件定义的容器和网络。

## 配置

默认配置使用内存存储。如果您需要持久化数据或其他存储类型（如 Elasticsearch），请修改 `docker-compose.yml` 文件中的 `STORAGE_TYPE` 环境变量和相关配置。

```yaml
# 示例：使用 Elasticsearch 存储
# environment:
#   - STORAGE_TYPE=elasticsearch
#   - ES_HOSTS=elasticsearch:9200
```
```
