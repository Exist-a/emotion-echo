#!/bin/bash

# 数据库迁移脚本

set -e

# 默认配置
DB_HOST=${DB_HOST:-localhost}
DB_PORT=${DB_PORT:-5432}
DB_USER=${DB_USER:-postgres}
DB_PASSWORD=${DB_PASSWORD:-}
DB_NAME=${DB_NAME:-emotion_echo}

export PGPASSWORD=$DB_PASSWORD

# 显示帮助
usage() {
    echo "Usage: $0 {up|down|create} [migration_name]"
    echo ""
    echo "Commands:"
    echo "  up              执行所有待执行的迁移"
    echo "  down            回滚最后一次迁移"
    echo "  create <name>   创建新的迁移文件"
    exit 1
}

# 执行迁移
migrate_up() {
    echo "执行数据库迁移..."
    for file in migrations/*.up.sql; do
        echo "执行: $file"
        psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -f "$file"
    done
    echo "迁移完成"
}

# 回滚迁移
migrate_down() {
    echo "回滚最后一次迁移..."
    # TODO: 实现回滚逻辑
}

# 创建新迁移
create_migration() {
    local name=$1
    if [ -z "$name" ]; then
        echo "错误: 请提供迁移名称"
        usage
    fi
    
    local timestamp=$(date +%Y%m%d%H%M%S)
    local up_file="migrations/${timestamp}_${name}.up.sql"
    local down_file="migrations/${timestamp}_${name}.down.sql"
    
    touch "$up_file"
    touch "$down_file"
    
    echo "创建迁移文件:"
    echo "  $up_file"
    echo "  $down_file"
}

# 主逻辑
case "$1" in
    up)
        migrate_up
        ;;
    down)
        migrate_down
        ;;
    create)
        create_migration "$2"
        ;;
    *)
        usage
        ;;
esac
