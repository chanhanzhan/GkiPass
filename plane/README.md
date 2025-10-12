# GKiPass 面板服务端 (Plane)

GKiPass 面板服务端是一个纯API服务器，提供节点管理、规则配置、隧道创建和监控等功能。

## 架构设计

面板服务端采用纯API设计，不提供Web服务，前端应用可以通过API与服务端进行交互。

### 主要组件

1. **API服务器**：提供RESTful API接口，用于节点管理、规则配置等功能。
2. **WebSocket服务**：提供实时通信，用于节点状态更新、规则同步等功能。
3. **数据库**：存储节点信息、规则配置、用户数据等。
4. **认证系统**：提供用户认证和授权功能。
5. **ACL系统**：提供访问控制功能，控制节点和用户的访问权限。

## API 接口

### 认证接口

- `POST /api/v1/auth/login`：用户登录
- `POST /api/v1/auth/logout`：用户登出
- `POST /api/v1/auth/refresh`：刷新令牌
- `POST /api/v1/auth/node`：节点认证

### 节点接口

- `GET /api/v1/nodes`：获取所有节点
- `POST /api/v1/nodes`：创建节点
- `GET /api/v1/nodes/{id}`：获取节点详情
- `PUT /api/v1/nodes/{id}`：更新节点
- `DELETE /api/v1/nodes/{id}`：删除节点
- `GET /api/v1/nodes/{id}/status`：获取节点状态
- `GET /api/v1/nodes/{id}/rules`：获取节点规则
- `POST /api/v1/nodes/{id}/rules/sync`：同步节点规则

### 节点组接口

- `GET /api/v1/node-groups`：获取所有节点组
- `POST /api/v1/node-groups`：创建节点组
- `GET /api/v1/node-groups/{id}`：获取节点组详情
- `PUT /api/v1/node-groups/{id}`：更新节点组
- `DELETE /api/v1/node-groups/{id}`：删除节点组
- `GET /api/v1/node-groups/{id}/nodes`：获取节点组中的节点
- `PUT /api/v1/node-groups/{id}/nodes/{nodeId}`：添加节点到组
- `DELETE /api/v1/node-groups/{id}/nodes/{nodeId}`：从组中移除节点
- `GET /api/v1/nodes/{id}/groups`：获取节点所属的组
- `GET /api/v1/node-groups/ingress`：获取所有入口组
- `GET /api/v1/node-groups/egress`：获取所有出口组

### 规则接口

- `GET /api/v1/rules`：获取所有规则
- `POST /api/v1/rules`：创建规则
- `GET /api/v1/rules/{id}`：获取规则详情
- `PUT /api/v1/rules/{id}`：更新规则
- `DELETE /api/v1/rules/{id}`：删除规则
- `GET /api/v1/rules/{id}/stats`：获取规则统计信息
- `POST /api/v1/rules/{id}/stats/reset`：重置规则统计信息

### ACL接口

- `GET /api/v1/rules/{rule_id}/acl`：获取规则的ACL规则
- `POST /api/v1/rules/{rule_id}/acl`：创建ACL规则
- `PUT /api/v1/rules/{rule_id}/acl/{id}`：更新ACL规则
- `DELETE /api/v1/rules/{rule_id}/acl/{id}`：删除ACL规则
- `POST /api/v1/rules/{rule_id}/acl/check`：检查访问权限

### 用户接口

- `GET /api/v1/users`：获取所有用户
- `POST /api/v1/users`：创建用户
- `GET /api/v1/users/{id}`：获取用户详情
- `PUT /api/v1/users/{id}`：更新用户
- `DELETE /api/v1/users/{id}`：删除用户
- `PUT /api/v1/users/{id}/password`：修改密码
- `GET /api/v1/users/{id}/permissions`：获取用户权限

### 监控接口

- `GET /api/v1/monitoring/nodes`：获取节点监控数据
- `GET /api/v1/monitoring/rules`：获取规则监控数据
- `GET /api/v1/monitoring/system`：获取系统监控数据

### WebSocket接口

- `GET /api/v1/ws/node`：节点WebSocket连接
- `GET /api/v1/ws/admin`：管理员WebSocket连接

## WebSocket协议

WebSocket协议用于实时通信，支持以下消息类型：

### 认证和连接消息

- `auth`：客户端认证
- `auth_result`：认证结果
- `ping`：心跳请求
- `pong`：心跳响应
- `disconnect`：断开连接

### 规则和配置消息

- `rule_sync`：规则同步
- `rule_sync_ack`：规则同步确认
- `config_update`：配置更新
- `config_update_ack`：配置更新确认

### 状态和监控消息

- `status_report`：状态报告
- `metrics_report`：指标报告
- `log_report`：日志报告

### 命令和控制消息

- `command`：命令
- `command_result`：命令结果

### 探测消息

- `probe_request`：探测请求
- `probe_result`：探测结果

### 错误和通知消息

- `error`：错误
- `notification`：通知

## 配置文件

配置文件使用YAML格式，包含以下内容：

```yaml
api:
  listen_addr: 0.0.0.0
  listen_port: 8080
  read_timeout: 15s
  write_timeout: 15s
  idle_timeout: 60s
  enable_tls: false
  tls_cert_file: ""
  tls_key_file: ""
  tls_client_ca: ""
  tls_min_version: "1.2"
  cors_enabled: true
  cors_allowed_origins: ["*"]
  log_requests: true
  log_level: "info"
  rate_limit_enabled: true
  rate_limit: 100
  rate_burst: 200
  rate_limit_by_ip: true

database:
  type: "sqlite"
  path: "./data/plane.db"
  max_open_conns: 10
  max_idle_conns: 5
  conn_max_lifetime: 1h

auth:
  jwt_secret: ""
  jwt_expiration: 24h
  refresh_token_expiration: 7d
  allow_api_key: true
  api_key_expiration: 90d

logging:
  level: "info"
  format: "json"
  output: "stdout"
  file_path: "./logs/plane.log"
  max_size: 100
  max_age: 30
  max_backups: 10
  compress: true
```

## 数据库模型

### 节点表 (nodes)

存储节点信息，包括节点ID、名称、状态等。

### 节点组表 (node_groups)

存储节点组信息，包括组名、角色、权限等。

### 节点组节点关联表 (node_group_nodes)

存储节点组和节点的关联关系。

### 规则表 (rules)

存储隧道规则，包括协议、端口、目标地址等。

### ACL规则表 (rule_acls)

存储ACL规则，控制访问权限。

### 规则选项表 (rule_options)

存储规则的额外选项。

### 用户表 (users)

存储用户信息，包括用户名、密码、角色等。

### 权限表 (permissions)

存储权限信息，控制用户的访问权限。

## 安装和运行

### 从源码构建

```bash
git clone https://github.com/yourusername/gkipass.git
cd gkipass/plane
go build -o plane ./cmd
```

### 运行

```bash
./plane --config config.yaml
```

## API 认证

API 认证支持两种方式：

1. **JWT 认证**：使用 Bearer Token 进行认证
2. **API 密钥认证**：使用 X-API-Key 头部进行认证

### JWT 认证

```
Authorization: Bearer <token>
```

### API 密钥认证

```
X-API-Key: <api_key>
```

## 开发指南

### 目录结构

```
plane/
├── cmd/            # 命令行入口
├── db/             # 数据库迁移和初始化
├── internal/       # 内部包
│   ├── api/        # API 处理器和中间件
│   ├── config/     # 配置
│   ├── model/      # 数据模型
│   ├── protocol/   # 协议定义
│   └── service/    # 业务逻辑
└── pkg/            # 公共包
```

### 添加新功能

1. 在 `internal/model` 中定义数据模型
2. 在 `internal/service` 中实现业务逻辑
3. 在 `internal/api/handler` 中实现 API 处理器
4. 在 `internal/api/server.go` 中注册路由

## 贡献指南

欢迎贡献代码、报告问题或提出建议。请遵循以下步骤：

1. Fork 仓库
2. 创建功能分支 (`git checkout -b feature/amazing-feature`)
3. 提交更改 (`git commit -m 'Add some amazing feature'`)
4. 推送到分支 (`git push origin feature/amazing-feature`)
5. 创建 Pull Request

## 许可证

[MIT](LICENSE)
