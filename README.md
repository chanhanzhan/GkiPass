# GKI Pass - 企业级双向隧道控制平台

<div align="center">

![Version](https://img.shields.io/badge/version-2.0.0-blue.svg)
![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go)
![Next.js](https://img.shields.io/badge/Next.js-15-black?logo=next.js)
![License](https://img.shields.io/badge/license-MIT-green.svg)

**基于 Go + Next.js 的企业级双向隧道代理管理系统**

[功能特性](#功能特性) • [快速开始](#快速开始) • [配置说明](#配置说明) • [API文档](#api文档) • [部署指南](#部署指南)

</div>

---

## ✨ 功能特性

### 核心功能
- 🔗 **隧道管理** - 创建和管理双向隧道规则
- 🖥️ **节点管理** - 入口/出口节点配置和监控
- 📊 **实时统计** - 流量、连接数、带宽监控
- 🔐 **证书管理** - CA/Leaf证书自动生成和管理
- 📦 **套餐系统** - 灵活的配额和限制管理

### 新增功能（v2.0.0）
- 🔐 **验证码系统** - 支持图片验证码和Cloudflare Turnstile
- 💰 **钱包系统** - 余额管理、充值、交易记录
- 🔔 **通知系统** - 个人通知和全局系统通知
- 📢 **公告系统** - 系统公告发布和管理
- ⚙️ **系统设置** - 动态配置验证码等功能
- 🎨 **现代化UI** - 玻璃态设计，渐变色主题

### 用户体验
- 🎨 **现代化界面** - 参考哪吒Stats的优秀设计
- 💬 **即时反馈** - Toast通知组件
- 📱 **响应式设计** - 完美适配各种设备
- 🌐 **防翻译保护** - 避免浏览器翻译导致的问题
- ⚡ **流畅动画** - 平滑的过渡效果

---

## 🚀 快速开始

### 环境要求
- Go 1.21+
- Node.js 18+
- Redis（可选，用于验证码和缓存）
- SQLite 3

### 安装步骤

1. **克隆项目**
```bash
git clone https://github.com/your/gkipass.git
cd gkipass
```

2. **启动后端**
```bash
cd plane
go mod download
go build -o ../bin/plane ./cmd

cd ../bin
./plane
```

3. **启动前端**
```bash
cd web
npm install
npm run dev
```

4. **访问系统**
- 前端：http://localhost:3000
- 后端API：http://localhost:8080
- 健康检查：http://localhost:8080/health

### 首次使用

1. 访问 http://localhost:3000
2. 点击「立即注册」创建账号
3. **第一个注册的用户将自动成为管理员** 👑
4. 登录后即可使用所有功能

---

## ⚙️ 配置说明

### 验证码配置

编辑 `bin/config.yaml`：

```yaml
captcha:
  enabled: true                    # 是否启用验证码
  type: image                      # 类型: image（图片）, turnstile（CF）
  enable_login: false              # 登录页面是否启用
  enable_register: true            # 注册页面是否启用
  image_width: 240                 # 图片验证码宽度
  image_height: 80                 # 图片验证码高度
  code_length: 6                   # 验证码长度
  expiration: 300                  # 过期时间（秒）
  turnstile_site_key: ""           # Turnstile站点密钥
  turnstile_secret_key: ""         # Turnstile服务端密钥
```

### JWT配置

```yaml
auth:
  jwt_secret: "change-this-in-production"  # ⚠️ 生产环境必须更改
  jwt_expiration: 24                        # Token有效期（小时）
```

### 数据库配置

```yaml
database:
  sqlite_path: ./data/gkipass.db           # SQLite数据库路径
  redis_addr: localhost:6379               # Redis地址
  redis_password: ""                       # Redis密码
  redis_db: 0                              # Redis数据库编号
```

---

## 📡 API文档

### 认证接口

```http
POST /api/v1/auth/register          # 用户注册
POST /api/v1/auth/login             # 用户登录
POST /api/v1/auth/logout            # 用户登出
```

### 验证码接口

```http
GET  /api/v1/captcha/config         # 获取验证码配置（公开）
GET  /api/v1/captcha/image          # 生成图片验证码（公开）
POST /api/v1/captcha/verify         # 验证验证码
```

### 钱包接口

```http
GET  /api/v1/wallet/balance         # 获取余额
GET  /api/v1/wallet/transactions    # 获取交易记录
POST /api/v1/wallet/recharge        # 充值
```

### 通知接口

```http
GET    /api/v1/notifications        # 获取通知列表
POST   /api/v1/notifications/:id/read       # 标记为已读
POST   /api/v1/notifications/read-all       # 全部标记为已读
DELETE /api/v1/notifications/:id            # 删除通知
```

### 公告接口

```http
# 用户端
GET  /api/v1/announcements          # 获取有效公告
GET  /api/v1/announcements/:id      # 获取公告详情

# 管理员端
GET    /api/v1/admin/announcements         # 获取所有公告
POST   /api/v1/admin/announcements         # 创建公告
PUT    /api/v1/admin/announcements/:id     # 更新公告
DELETE /api/v1/admin/announcements/:id     # 删除公告
```

### 系统设置接口（管理员）

```http
GET /api/v1/admin/settings/captcha  # 获取验证码设置
PUT /api/v1/admin/settings/captcha  # 更新验证码设置
```

完整API文档请参考：[API_DOCUMENTATION.md](./API_DOCUMENTATION.md)

---

## 🎨 UI设计

### 设计风格
- **主题**：Glassmorphism（玻璃态）
- **配色**：蓝色-紫色-粉色渐变
- **圆角**：16-24px（rounded-xl/2xl）
- **阴影**：多层次阴影效果
- **动画**：平滑过渡（duration-200/300）

### 颜色方案
```css
--primary-blue: #3b82f6
--primary-purple: #a855f7  
--primary-pink: #ec4899
--gradient-main: linear-gradient(to right, #3b82f6, #a855f7, #ec4899)
```

### 参考设计
本项目UI参考了 [哪吒Stats](https://github.com/nezhahq/nezha) 的优秀设计。

---

## 🏗️ 项目结构

```
gkipass/
├── bin/                      # 编译输出
│   ├── plane                 # 后端可执行文件
│   ├── config.yaml           # 配置文件
│   └── data/                 # 数据目录
├── plane/                    # 后端（Go）
│   ├── cmd/                  # 入口
│   ├── db/                   # 数据库层
│   │   ├── cache/            # Redis缓存
│   │   ├── sqlite/           # SQLite操作
│   │   ├── init/             # 数据结构定义
│   │   └── migrations/       # 数据库迁移
│   ├── internal/
│   │   ├── api/              # API处理器
│   │   │   ├── handler_*.go  # 各功能Handler
│   │   │   ├── middleware/   # 中间件
│   │   │   └── router.go     # 路由配置
│   │   ├── config/           # 配置管理
│   │   └── service/          # 业务逻辑
│   └── pkg/                  # 公共包
└── web/                      # 前端（Next.js）
    ├── src/
    │   ├── app/              # 页面
    │   │   ├── (auth)/       # 认证页面
    │   │   └── (dashboard)/  # 仪表板页面
    │   ├── components/       # 组件
    │   │   ├── layout/       # 布局组件
    │   │   └── ui/           # UI组件
    │   ├── hooks/            # React Hooks
    │   ├── lib/              # 工具库
    │   └── types/            # TypeScript类型
    └── public/               # 静态文件
```

---

## 🔒 安全特性

- ✅ JWT令牌认证
- ✅ 密码bcrypt加密
- ✅ 验证码防机器人攻击
- ✅ CORS跨域保护
- ✅ SQL注入防护
- ✅ XSS防护
- ✅ 验证码一次性使用
- ✅ Token自动过期

---

## 📖 使用文档

### 用户功能
- 注册和登录（支持验证码）
- 创建和管理隧道
- 查看节点状态
- 订阅套餐
- 查看钱包余额和交易记录
- 接收系统通知
- 查看系统公告

### 管理员功能
- 用户管理
- 节点管理
- 套餐管理
- 公告发布和管理
- 系统通知推送
- 验证码配置
- 数据统计和监控

---

## 🤝 贡献指南

欢迎提交Issue和Pull Request！

### 开发流程
1. Fork本仓库
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 开启Pull Request

---

## 📄 许可证

本项目采用 MIT 许可证。详见 [LICENSE](LICENSE) 文件。

---

## 🙏 致谢

- [Gin](https://github.com/gin-gonic/gin) - Go Web框架
- [Next.js](https://nextjs.org/) - React框架
- [Tailwind CSS](https://tailwindcss.com/) - CSS框架
- [base64Captcha](https://github.com/mojocn/base64Captcha) - Go验证码库
- [Zustand](https://github.com/pmndrs/zustand) - 状态管理
- [哪吒Stats](https://github.com/nezhahq/nezha) - UI设计灵感

---

## 📞 联系方式

- 项目地址：https://github.com/your/gkipass
- 问题反馈：https://github.com/your/gkipass/issues

---

<div align="center">

**Made with ❤️ by GKI Pass Team**

© 2025 GKI Pass. All rights reserved.

</div>
