# 文档编辑器 (Doc Site Go)

一个基于 Go 语言的轻量级在线文档编辑器，支持用户认证、文档 CRUD 操作和实时编辑。

## 🚀 功能特性

- **用户认证** - JWT Token 认证，安全可靠
- **文档管理** - 创建、读取、更新、删除文档
- **实时编辑** - 简洁的在线编辑器界面
- **响应式设计** - 适配桌面和移动端
- **安全加固** - 安全头、CORS、XSS 防护

## 🛠️ 技术栈

- **后端**: Go 1.21+
- **前端**: HTML5 + CSS3 + JavaScript
- **反向代理**: Nginx
- **认证**: JWT (JSON Web Token)

## 📦 快速开始

### 环境要求

- Go 1.21 或更高版本
- Nginx (可选，用于生产环境)

### 本地开发

```bash
# 克隆项目
git clone https://github.com/qxiaoxia/doc-site-go.git
cd doc-site-go

# 安装依赖
go mod download

# 运行后端服务
go run cmd/main.go

# 访问编辑器
# 浏览器打开 http://localhost:3000
```

### 生产部署

```bash
# 编译
go build -o doc-server cmd/main.go

# 启动服务
./doc-server

# Nginx 配置 (参考 /etc/nginx/sites-available/doc-site)
# 反向代理 /doc/api/ 到 http://127.0.0.1:3000
```

## 🔐 默认配置

| 配置项 | 值 |
|--------|-----|
| 默认账号 | `admin` |
| 默认密码 | `admin123` |
| 服务端口 | `3000` |
| JWT 密钥 | (请在配置文件中修改) |

## 📁 项目结构

```
doc-site-go/
├── cmd/
│   ├── main.go      # 主程序入口
│   └── debug.go     # 调试工具
├── internal/
│   ├── auth.go      # 认证模块
│   ├── handler.go   # HTTP 处理器
│   └── storage.go   # 文档存储
├── static/          # 静态资源
├── go.mod           # Go 模块定义
├── go.sum           # 依赖校验
└── .gitignore       # Git 忽略配置
```

## 🔌 API 接口

### 认证接口

```bash
# 登录
POST /api/login
Content-Type: application/json

{
  "username": "admin",
  "password": "admin123"
}

# 响应
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires": "2026-03-11T09:00:00Z"
}
```

### 文档接口

```bash
# 获取文档列表
GET /api/documents
Authorization: Bearer <token>

# 创建文档
POST /api/documents
Authorization: Bearer <token>
Content-Type: application/json

{
  "title": "我的文档",
  "content": "文档内容..."
}

# 更新文档
PUT /api/documents/:id
Authorization: Bearer <token>

# 删除文档
DELETE /api/documents/:id
Authorization: Bearer <token>
```

## 🌐 在线访问

**本地开发**:
- Go 后端：`http://localhost:3000` (仅本地)
- Nginx 代理：`http://localhost:62739/doc/login.html`

**生产部署**:
- Go 后端监听 `127.0.0.1:3000` (不对外暴露)
- Nginx 监听高位随机端口 (如 `62739`)，通过防火墙控制访问
- 建议使用 SSH 隧道或内网穿透访问

## ⚙️ 配置说明

配置文件 `config.json` (需自行创建):

```json
{
  "port": 3000,
  "jwtSecret": "your-secret-key-change-in-production",
  "dataDir": "./data"
}
```

## 🔒 安全建议

1. **修改默认密码** - 首次登录后立即修改
2. **配置 JWT 密钥** - 使用强随机密钥
3. **启用 HTTPS** - 生产环境必须使用 SSL
4. **限制访问** - 配置防火墙和访问控制

## 📝 开发日志

- 2026-03-09: 初始版本发布
  - 用户认证系统
  - 文档 CRUD API
  - 基础编辑器界面

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📄 许可证

MIT License

---

**作者**: qxiaoxia  
**GitHub**: https://github.com/qxiaoxia/doc-site-go
