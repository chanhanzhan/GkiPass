.PHONY: build run test clean install deps

# 变量
BINARY_NAME=gkipass-client
GO=go
GOFLAGS=-v
LDFLAGS=-ldflags "-s -w"

# 默认目标
all: build

# 安装依赖
deps:
	$(GO) mod download
	$(GO) mod tidy

# 编译
build: deps
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o bin/$(BINARY_NAME) cmd/main.go

# 运行
run: build
	./bin/$(BINARY_NAME) -config=config.yaml

# 开发模式运行（不编译）
dev:
	$(GO) run cmd/main.go -config=config.yaml

# 测试
test:
	$(GO) test $(GOFLAGS) ./...

# 测试覆盖率
test-coverage:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out

# 清理
clean:
	rm -rf bin/
	rm -f coverage.out

# 安装到系统
install: build
	cp bin/$(BINARY_NAME) /usr/local/bin/

# 交叉编译
build-linux:
	GOOS=linux GOARCH=amd64 $(GO) build $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-amd64 cmd/main.go

build-windows:
	GOOS=windows GOARCH=amd64 $(GO) build $(LDFLAGS) -o bin/$(BINARY_NAME)-windows-amd64.exe cmd/main.go

build-darwin:
	GOOS=darwin GOARCH=amd64 $(GO) build $(LDFLAGS) -o bin/$(BINARY_NAME)-darwin-amd64 cmd/main.go

build-all: build-linux build-windows build-darwin

# 代码检查
lint:
	golangci-lint run

# 格式化代码
fmt:
	$(GO) fmt ./...
	goimports -w .

# 查看帮助
help:
	@echo "GKI Pass Client - Makefile 帮助"
	@echo ""
	@echo "可用目标:"
	@echo "  make build         - 编译客户端"
	@echo "  make run           - 编译并运行"
	@echo "  make dev           - 开发模式运行（不编译）"
	@echo "  make test          - 运行测试"
	@echo "  make test-coverage - 测试覆盖率"
	@echo "  make clean         - 清理编译文件"
	@echo "  make deps          - 安装依赖"
	@echo "  make lint          - 代码检查"
	@echo "  make fmt           - 格式化代码"
	@echo "  make build-all     - 交叉编译所有平台"







