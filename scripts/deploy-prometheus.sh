#!/bin/bash

# Prometheus 部署脚本
# 仅支持 Docker 部署方式

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 默认部署方式
DEPLOY_TYPE=${1:-docker}

echo -e "${GREEN}开始部署 Prometheus...${NC}"
echo -e "${YELLOW}部署方式: $DEPLOY_TYPE${NC}"

# 检查依赖
check_dependencies() {
    if [[ "$DEPLOY_TYPE" == "docker" ]]; then
        if ! command -v docker &> /dev/null; then
            echo -e "${RED}错误: Docker 未安装${NC}"
            exit 1
        fi
        if ! command -v docker-compose &> /dev/null; then
            echo -e "${RED}错误: Docker Compose 未安装${NC}"
            exit 1
        fi
    else
        echo -e "${RED}错误: 仅支持 Docker 部署方式${NC}"
        echo -e "${YELLOW}支持的部署方式: docker${NC}"
        exit 1
    fi
}

# Docker 部署
deploy_docker() {
    echo -e "${GREEN}使用 Docker Compose 部署 Prometheus...${NC}"
    
    cd ./deployments/prometheus
    
    # 启动服务
    echo -e "${YELLOW}启动 Prometheus 服务...${NC}"
    docker-compose up -d
    
    # 等待服务启动
    echo -e "${YELLOW}等待服务启动...${NC}"
    sleep 10
    
    # 检查服务状态
    echo -e "${YELLOW}检查服务状态...${NC}"
    docker-compose ps
    
    echo -e "${GREEN}Docker 部署完成！${NC}"
    echo -e "${YELLOW}访问地址:${NC}"
    echo -e "  Prometheus: http://localhost:9090"
    echo -e "  Grafana: http://localhost:3000 (admin/admin)"
}

# 清理函数
cleanup() {
    echo -e "${YELLOW}清理资源...${NC}"
    cd ./deployments/prometheus
    docker-compose down -v
    echo -e "${GREEN}清理完成！${NC}"
}

# 显示帮助
show_help() {
    echo "用法: $0 [docker|cleanup|help]"
    echo ""
    echo "参数:"
    echo "  docker      使用 Docker Compose 部署 Prometheus"
    echo "  cleanup     清理所有部署的资源"
    echo "  help        显示此帮助信息"
    echo ""
    echo "示例:"
    echo "  $0 docker      # Docker 部署"
    echo "  $0 cleanup     # 清理资源"
}

# 主函数
main() {
    case "$DEPLOY_TYPE" in
        docker)
            check_dependencies
            deploy_docker
            ;;
        cleanup)
            cleanup
            ;;
        help|--help|-h)
            show_help
            ;;
        *)
            echo -e "${RED}错误: 未知的部署方式 '$DEPLOY_TYPE'${NC}"
            show_help
            exit 1
            ;;
    esac
}

# 执行主函数
main