.PHONY: help build run test clean migrate

# 默认目标
help:
	@echo "Available targets:"
	@echo "  build     - 构建项目"
	@echo "  run       - 运行项目"
	@echo "  test      - 运行测试"
	@echo "  clean     - 清理构建文件"
	@echo "  migrate   - 执行数据库迁移"
	@echo "  deps      - 安装依赖"

# 构建
build:
	go build -o bin/server cmd/server/main.go

# 运行
run:
	go run cmd/server/main.go

# 测试
test:
	go test -v ./...

# 清理
clean:
	rm -rf bin/
	go clean

# 数据库迁移
migrate:
	bash scripts/migrate.sh up

# 安装依赖
deps:
	go mod download
	go mod tidy

# 格式化代码
fmt:
	go fmt ./...

# 代码检查
lint:
	golangci-lint run

# 生成 swagger 文档
swagger:
	swag init -g cmd/server/main.go
