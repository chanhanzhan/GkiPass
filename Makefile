.PHONY: help build run clean test install deps plane-build plane-run

# 默认目标
help:
	@echo "GKI Pass - Makefile Commands"
	@echo ""
	@echo "  make deps          - 安装 Go 依赖"
	@echo "  make plane-build   - 编译控制面板后端"
	@echo "  make plane-run     - 运行控制面板后端"
	@echo "  make build         - 编译所有组件"
	@echo "  make run           - 运行控制面板"
	@echo "  make clean         - 清理编译文件"
	@echo "  make test          - 运行测试"
	@echo ""

# 安装依赖
deps:
	@echo "📦 安装 Go 依赖..."
	go mod download
	go mod tidy
	@echo "✓ 依赖安装完成"

# 编译控制面板
plane-build:
	@echo "🔨 编译控制面板..."
	go build -ldflags="-s -w" -o bin/gkipass-plane ./plane/cmd
	@echo "✓ 编译完成: bin/gkipass-plane"

# 运行控制面板
plane-run:
	@echo "🚀 启动控制面板..."
	go run ./plane/cmd

# 编译所有组件
build: deps plane-build
	@echo "✓ 所有组件编译完成"

# 运行（默认运行控制面板）
run: plane-run

# 清理编译文件
clean:
	@echo "🧹 清理编译文件..."
	rm -rf bin/
	rm -f gkipass-*
	rm -rf data/*.db*
	@echo "✓ 清理完成"

# 运行测试
test:
	@echo "🧪 运行测试..."
	go test -v ./...

# 初始化项目
init: deps
	@echo "📁 创建必要的目录..."
	mkdir -p data certs logs
	@echo "📝 复制配置文件..."
	@if [ ! -f config.yaml ]; then cp config.example.yaml config.yaml; fi
	@echo "✓ 项目初始化完成"

# 交叉编译
build-linux:
	@echo "🐧 编译 Linux 版本..."
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/gkipass-plane-linux plane/cmd/main.go
	@echo "✓ Linux 版本编译完成"

build-windows:
	@echo "🪟 编译 Windows 版本..."
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o bin/gkipass-plane.exe plane/cmd/main.go
	@echo "✓ Windows 版本编译完成"

build-mac:
	@echo "🍎 编译 macOS 版本..."
	GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o bin/gkipass-plane-mac plane/cmd/main.go
	@echo "✓ macOS 版本编译完成"

build-all: build-linux build-windows build-mac
	@echo "✓ 所有平台编译完成"

# 开发模式（热重载需要额外工具）
dev:
	@echo "🔧 开发模式..."
	@if command -v air > /dev/null; then \
		air -c .air.toml; \
	else \
		echo "⚠️  未安装 air，使用普通运行模式"; \
		echo "安装 air: go install github.com/air-verse/air@latest"; \
		go run plane/cmd/main.go; \
	fi

# Docker 构建（预留）
docker-build:
	@echo "🐳 构建 Docker 镜像..."
	docker build -t gkipass-plane:latest -f Dockerfile.plane .

# 格式化代码
fmt:
	@echo "🎨 格式化代码..."
	go fmt ./...
	@echo "✓ 格式化完成"

# 代码检查
lint:
	@echo "🔍 代码检查..."
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run; \
	else \
		echo "⚠️  未安装 golangci-lint"; \
		echo "安装: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi


