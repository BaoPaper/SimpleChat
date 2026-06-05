# AGENTS.md — SimpleChat 开发指南

## 核心哲学：保持简单

一切以"简单"为最高原则。能用配置文件解决的问题绝不写代码，能用标准库的绝不引入第三方依赖。这个项目的目标是让每个人都能在五分钟内跑起来自己的 LLM 聊天界面。

---

## 一、配置文件是第一公民

**原则：一切配置都放在人类可读、可编辑的 JSON 配置文件中，而不藏在代码、数据库或环境变量里。**

```
config/
├── settings.json   ← 用户列表、密码、端口、JWT密钥、系统提示词
└── models.json     ← API地址、API Key、可用模型列表
```

- 所有可变的参数都走配置文件，不在代码中硬编码
- 首次运行时自动生成带默认值的配置文件（含随机 JWT 密钥）
- 配置只在启动时加载一次——刻意不引入热重载，保持简单
- `models.json` 可以配置任意 OpenAI-compatible 的 API（OpenAI / DeepSeek / Ollama / vLLM 等）

### settings.json 设计

```json
{
  "port": 8080,
  "users": [
    { "username": "admin", "password": "admin" },
    { "username": "guest", "password": "guest123" }
  ],
  "jwt_secret": "auto-generated-random-hex",
  "database_path": "./data/simplechat.db",
  "system_prompt": "你是一个有用的 AI 助手。"
}
```

用户直接在 `settings.json` 的 `users` 数组中定义，支持多个用户名密码对。

---

## 二、最小化数据库使用

**原则：SQLite 只用来存会变化的数据——会话列表和聊天消息。其余一切都放在配置文件中。**

数据库表极简设计（两张表）：

| 表 | 字段 | 用途 |
|----|------|------|
| `sessions` | id, user_id, title, created_at, updated_at | 会话列表 |
| `messages` | id, session_id, role, content, created_at | 聊天消息 |

**不要放进数据库的东西：**
- 用户和密码（放在 `settings.json`）
- API Key 和密钥（放在 `models.json`）
- 模型选择偏好（放在客户端 localStorage）
- 主题设置（放在客户端 localStorage）
- 系统提示词（放在 `settings.json`）

**判断标准：** 如果一份数据更适合人类手动编辑，那它就不应该进数据库。

---

## 三、技术栈

```
SimpleChat/
├── backend/                 ← Go 后端（独立模块）
│   ├── main.go              ← 入口 + 路由 + 内嵌前端
│   ├── config.go            ← 配置文件加载 + 自动生成
│   ├── db.go                ← SQLite 操作
│   ├── embed.go             ← //go:embed frontend/*
│   ├── handler.go           ← 所有 HTTP 处理器
│   ├── middleware.go         ← JWT 鉴权
│   ├── openai.go            ← OpenAI-compatible 流式客户端
│   ├── go.mod / go.sum
│   └── frontend/            ← 前端源文件（编译时内嵌）
│       ├── index.html
│       ├── script.js
│       └── styles.css
├── scripts/                 ← 构建脚本
│   └── download-deps.sh     ← 下载前端依赖（marked.js, highlight.js 等）
├── config/                  ← 运行时自动生成，用户编辑
├── Dockerfile               ← 多阶段构建
├── docker-compose.yml
└── .gitignore
```

- 后端：Go + Gin + SQLite + JWT
- 前端：纯 HTML + CSS + JS，通过 `//go:embed` 内嵌到二进制
- 前端依赖（marked.js, highlight.js 等）通过构建脚本下载到本地，不走 CDN
- 部署：Docker 多阶段构建，一个镜像即可运行

---

## 四、API 端点

所有 API 以 `/api` 开头，鉴权用 `Authorization: Bearer <JWT>` 头，聊天用 SSE 流式返回。

| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| POST | `/api/login` | 否 | 用户名+密码登录，返回 JWT |
| GET | `/api/check` | 是 | 验证 Token |
| GET | `/api/models` | 是 | 获取模型列表 |
| GET | `/api/sessions` | 是 | 获取当前用户的会话列表 |
| POST | `/api/sessions` | 是 | 创建新会话 |
| GET | `/api/sessions/:id` | 是 | 获取会话+消息 |
| PUT | `/api/sessions/:id` | 是 | 重命名会话 |
| DELETE | `/api/sessions/:id` | 是 | 删除会话 |
| POST | `/api/chat` | 是 | 发送消息（SSE流式） |

---

## 五、代码修改守则

### 应当遵循的惯例

- 配置项一律放 JSON 文件，不走代码硬编码
- 新功能优先考虑"能否通过配置文件实现"
- 发生功能、配置、API、部署方式或使用流程变更时，必须同步更新相关文档（如 `AGENTS.md`、`README.md`、示例配置、部署说明等）
- 使用中文写 commit 消息和文档
- 数据该放数据库放数据库，该放配置文件放配置文件——选择的标准是"用户会不会手动编辑它"

### 不提交的内容

- `config/` 目录（自动生成，含敏感信息）
- 二进制文件
- `data/` 目录
