# GKI Pass Client Node

GKI Pass 双向隧道系统的节点端实现。

## 🚀 快速开始

### 安装依赖

```bash
cd client
make deps
```

### 配置

编辑 `config.yaml`:

```yaml
node:
  name: "client-node-01"
  type: "entry"          # entry 或 exit
  group_id: ""           # 在Plane中创建的节点组ID

plane:
  url: "ws://localhost:8080/ws/node"  # Plane地址
  ck: ""                 # Connection Key（首次注册后自动保存）
```

### 运行

```bash
# 最简启动（仅需服务器和token）
./bin/gkipass-client --token <your-token> -s ws://plane:8080

# Entry节点
./bin/gkipass-client --token <token> -s ws://plane:8080 --type entry

# Exit节点
./bin/gkipass-client --token <token> -s ws://plane:8080 --type exit

# 启用TLS（节点间加密，0-RTT加速）
./bin/gkipass-client --token <token> -s ws://plane:8080 --tls

# 调试模式
./bin/gkipass-client --token <token> -s ws://plane:8080 --log debug
```

**特性**:
- ✅ 无需配置文件
- ✅ 配置由后端下发
- ✅ 支持0-RTT快速启动
- ✅ 自动协议识别

## 📋 功能状态

### ✅ 已实现（P0阶段 - MVP）

- [x] 配置管理（YAML加载、验证、自动生成节点ID）
- [x] 日志系统（Zap、结构化日志、多格式输出）
- [x] WebSocket客户端（连接、自动重连、心跳）
- [x] 节点注册与认证（CK认证、能力声明）
- [x] 规则同步机制（接收、解析、应用隧道规则）
- [x] **隧道管理器**（动态监听器、规则热更新）
- [x] **TCP流量转发**（Entry→Target直通、双向数据转发）
- [x] **流量统计**（按隧道统计、定期上报）
- [x] 优雅关闭（信号处理、资源清理）

### 🎯 当前可用功能

✅ **完整的隧道功能**：
- Entry节点监听端口
- 接收用户连接
- 连接目标服务器
- 双向流量转发
- 实时流量统计
- 心跳和状态上报

### 📅 待实现（P1-P3阶段）

- [ ] UDP支持（五元组会话映射）
- [ ] 协议识别（HTTP/TLS/SOCKS检测）
- [ ] 连接池管理（Entry⇄Exit长连接）
- [ ] 多路复用（单连接多stream）
- [ ] 流量整形（令牌桶限速）
- [ ] TLS加密（自签名证书）
- [ ] 负载均衡（多目标权重分配）

## 🏗️ 项目结构

```
client/
├── cmd/
│   └── main.go          # 入口文件
├── config/
│   ├── config.go        # 配置管理
│   └── config.yaml      # 配置文件
├── logger/
│   └── logger.go        # 日志模块
├── ws/
│   ├── client.go        # WebSocket客户端
│   ├── handler.go       # 消息处理器
│   └── protocol.go      # 协议定义
├── core/                # 核心业务逻辑（待实现）
├── pool/                # 连接池（待实现）
├── protocol/            # 协议处理（待实现）
├── metrics/             # 指标收集（待实现）
├── go.mod
├── Makefile
└── README.md
```

## 🔧 开发

### 编译

```bash
make build
```

### 运行测试

```bash
make test
```

### 代码格式化

```bash
make fmt
```

### 清理

```bash
make clean
```

### 交叉编译

```bash
make build-all
```

## 📖 使用说明

### 1. 首次运行

首次运行时，程序会：
- 自动生成唯一的节点ID
- 保存到配置文件

### 2. 注册节点

节点启动后会自动向Plane注册：
- 发送节点信息（ID、名称、类型、组ID）
- 等待注册确认
- 接收隧道规则

### 3. 心跳

节点每30秒发送一次心跳，包含：
- CPU使用率
- 内存使用
- 连接数

### 4. 日志

日志默认输出到stdout，支持：
- JSON格式
- 控制台格式
- 文件输出

## 🔐 安全

- WebSocket连接使用CK（Connection Key）认证
- 支持TLS加密（可选）
- 支持证书Pin校验（可选）

## 📊 监控

- 实时心跳状态
- 流量统计
- 连接数统计
- 错误日志

## 🐛 故障排查

### 连接失败

检查：
1. Plane地址是否正确
2. CK是否有效
3. 网络连接
4. 防火墙设置

### 注册失败

检查：
1. CK是否配置
2. 节点组ID是否正确
3. 查看Plane日志

## 📝 待办事项

详见 [CLIENT_TODO.md](../CLIENT_TODO.md)

## 🤝 贡献

欢迎提交Issue和Pull Request！

## 📄 许可

MIT License

## 📞 联系

- 项目地址：https://github.com/your/gkipass
- 文档：https://docs.gkipass.com

