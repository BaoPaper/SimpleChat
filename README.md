<!-- PROJECT SHIELDS -->

[![Contributors][contributors-shield]][contributors-url]
[![Forks][forks-shield]][forks-url]
[![Stargazers][stars-shield]][stars-url]
[![Issues][issues-shield]][issues-url]
[![MIT License][license-shield]][license-url]

<!-- PROJECT LOGO -->
<br />

<p align="center">
  <a href="https://github.com/BaoPaper/SimpleChat/">
    <img src="backend/frontend/favicon.svg" alt="Logo" width="80" height="80">
  </a>

  <h3 align="center">SimpleChat / 简单聊天</h3>
  <p align="center">
    💬 一个基于 Go + Gin + SQLite 的简洁 LLM 聊天界面
    <br />
    <br />
    <a href="https://github.com/BaoPaper/SimpleChat/issues">报告 Bug</a>
    ·
    <a href="https://github.com/BaoPaper/SimpleChat/issues">提出新特性</a>
  </p>
</p>

## 功能特性

- **多用户登录** - 用户名和密码直接写在 `config/settings.json`
- **OpenAI-compatible API** - 支持 OpenAI、DeepSeek、Ollama、vLLM 等兼容接口
- **多模型选择** - 模型列表写在 `config/models.json`，前端可切换
- **流式输出** - 使用 SSE 实时显示 AI 回复
- **断线恢复** - 生成中的回复会持久化到 SQLite，刷新页面后可继续显示
- **会话管理** - 支持新建、重命名、删除会话
- **消息编辑** - 支持编辑用户消息并重新生成后续回复
- **重新生成** - 支持对 AI 回复重新生成
- **Markdown 渲染** - 前端使用 Marked.js + DOMPurify 渲染和净化内容
- **代码高亮** - 使用 highlight.js 渲染代码块
- **纯前端页面** - 无前端构建工具，HTML/CSS/JS 编译时内嵌到 Go 二进制
- **Docker 部署** - 一个镜像即可运行，配置和数据目录挂载到宿主机

## 项目理念

SimpleChat 的核心目标是：**保持简单，五分钟内跑起来自己的 LLM 聊天界面。**

- 配置优先放 JSON 文件，不藏在环境变量或数据库里
- SQLite 只存会变化的数据：会话和消息
- 不做配置热重载，配置启动时加载一次
- 不引入复杂任务系统、ORM 或前端框架
- 前端依赖下载到本地并内嵌，不依赖 CDN

## 项目结构

```txt
SimpleChat/
├── AGENTS.md                 # 项目开发指南
├── Dockerfile                # 多阶段 Docker 构建
├── Makefile                  # 依赖下载、构建、运行脚本
├── docker-compose.yml        # Docker Compose 部署配置
├── backend/                  # Go 后端
│   ├── config.go             # JSON 配置加载与默认配置生成
│   ├── db.go                 # SQLite 初始化、迁移和数据操作
│   ├── embed.go              # 内嵌前端文件
│   ├── handler.go            # HTTP API 处理器
│   ├── main.go               # 程序入口、路由注册、静态资源服务
│   ├── middleware.go         # JWT 鉴权
│   ├── openai.go             # OpenAI-compatible 流式客户端
│   ├── go.mod
│   ├── go.sum
│   └── frontend/             # 前端源码
│       ├── favicon.svg
│       ├── index.html
│       ├── script.js
│       └── styles.css
├── config/                   # 运行时自动生成，用户编辑，不提交
│   ├── settings.json         # 用户、端口、JWT 密钥、数据库路径、系统提示词
│   └── models.json           # API 地址、API Key、模型列表
└── data/                     # SQLite 数据目录，不提交
    └── simplechat.db
```

## 安装与运行

### 方式一：本地运行

要求：

- Go 1.26+
- SQLite CGO 编译环境
- curl

1. 克隆仓库

```sh
git clone https://github.com/BaoPaper/SimpleChat.git
cd SimpleChat
```

2. 下载前端依赖并构建

```sh
make build
```

`make build` 会自动下载：

- Marked.js
- highlight.js
- DOMPurify
- highlight.js 主题 CSS

3. 首次运行

```sh
./simplechat
```

首次运行会自动生成：

```txt
config/settings.json
config/models.json
```

由于默认 `models.json` 中的 API Key 是占位值，首次运行通常会提示你编辑配置文件。  
编辑完成后重新启动即可。

4. 修改模型配置

打开：

```sh
config/models.json
```

示例：

```json
{
  "default_model": "gpt-4o-mini",
  "api_base": "https://api.openai.com/v1",
  "api_key": "sk-your-api-key",
  "models": [
    {
      "id": "gpt-4o-mini",
      "name": "GPT-4o Mini"
    },
    {
      "id": "gpt-4o",
      "name": "GPT-4o"
    }
  ]
}
```

5. 重新运行

```sh
./simplechat
```

默认访问地址：

```txt
http://localhost:8080
```

默认账号：

```txt
admin / admin
```

建议首次运行后立刻修改 `config/settings.json` 中的默认密码。

---

### 方式二：开发模式运行

```sh
make run
```

等价于：

```sh
make deps
make build
./simplechat
```

---

### 方式三：Docker Compose 运行

1. 创建配置和数据目录

```sh
mkdir -p config data
```

2. 启动服务

```sh
docker compose up -d --build
```

3. 查看日志

```sh
docker compose logs -f
```

4. 首次启动后编辑配置

首次启动会生成：

```txt
config/settings.json
config/models.json
```

编辑 `config/models.json`，填入你的 API 地址和 API Key，然后重启：

```sh
docker compose restart
```

5. 访问服务

```txt
http://localhost:8080
```

如果需要修改宿主机端口：

```sh
PORT=3000 docker compose up -d
```

然后访问：

```txt
http://localhost:3000
```

## 配置文件

SimpleChat 不依赖复杂环境变量，主要通过 JSON 文件配置。

### `config/settings.json`

```json
{
  "port": 8080,
  "users": [
    {
      "username": "admin",
      "password": "admin"
    }
  ],
  "jwt_secret": "auto-generated-random-hex",
  "database_path": "./data/simplechat.db",
  "system_prompt": "你是一个有用的 AI 助手。"
}
```

字段说明：

| 字段 | 说明 |
| ---- | ---- |
| `port` | 服务监听端口 |
| `users` | 用户列表，支持多个用户名密码 |
| `jwt_secret` | JWT 签名密钥，首次运行自动随机生成 |
| `database_path` | SQLite 数据库路径 |
| `system_prompt` | 全局系统提示词 |

添加多个用户：

```json
{
  "users": [
    {
      "username": "admin",
      "password": "admin"
    },
    {
      "username": "guest",
      "password": "guest123"
    }
  ]
}
```

### `config/models.json`

```json
{
  "default_model": "gpt-4o-mini",
  "api_base": "https://api.openai.com/v1",
  "api_key": "sk-your-api-key",
  "models": [
    {
      "id": "gpt-4o-mini",
      "name": "GPT-4o Mini"
    },
    {
      "id": "gpt-4o",
      "name": "GPT-4o"
    }
  ]
}
```

字段说明：

| 字段 | 说明 |
| ---- | ---- |
| `default_model` | 默认模型 ID，必须存在于 `models` 列表中 |
| `api_base` | OpenAI-compatible API 地址 |
| `api_key` | API Key，本地模型无需密钥时可设为空字符串 |
| `models` | 前端可选择的模型列表 |

### Ollama 示例

如果你使用 Ollama 的 OpenAI-compatible 接口，可以类似这样配置：

```json
{
  "default_model": "llama3.1",
  "api_base": "http://localhost:11434/v1",
  "api_key": "",
  "models": [
    {
      "id": "llama3.1",
      "name": "Llama 3.1"
    },
    {
      "id": "qwen2.5",
      "name": "Qwen 2.5"
    }
  ]
}
```

如果使用 Docker 部署，并且 Ollama 跑在宿主机上，`api_base` 可能需要写成：

```json
"api_base": "http://host.docker.internal:11434/v1"
```

Linux 环境下如不可用，可按实际网络配置改成宿主机 IP。

## API 接口

所有 API 均以 `/api` 开头。

鉴权方式：

```http
Authorization: Bearer <JWT>
```

聊天接口使用 SSE 流式返回。

| 方法 | 路径 | 鉴权 | 说明 |
| ---- | ---- | ---- | ---- |
| POST | `/api/login` | 否 | 登录，返回 JWT |
| GET | `/api/check` | 是 | 检查 Token 是否有效 |
| GET | `/api/models` | 是 | 获取模型列表 |
| GET | `/api/sessions` | 是 | 获取当前用户的会话列表 |
| POST | `/api/sessions` | 是 | 创建新会话 |
| GET | `/api/sessions/:id` | 是 | 获取会话和消息 |
| PUT | `/api/sessions/:id` | 是 | 重命名会话 |
| DELETE | `/api/sessions/:id` | 是 | 删除会话 |
| POST | `/api/chat` | 是 | 发送消息，SSE 流式返回 |
| PUT | `/api/chat/edit/:message_id` | 是 | 修改用户消息并重新生成 |
| POST | `/api/chat/regenerate/:message_id` | 是 | 重新生成 AI 回复 |

### 登录示例

```sh
curl -X POST http://localhost:8080/api/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin"}'
```

返回：

```json
{
  "token": "jwt-token",
  "username": "admin"
}
```

### 发送消息示例

```sh
curl -N -X POST http://localhost:8080/api/chat \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <JWT>' \
  -d '{
    "session_id": "",
    "model": "gpt-4o-mini",
    "message": "你好"
  }'
```

SSE 返回示例：

```txt
data: {"type":"meta","session_id":"...","assistant_message_id":1,"user_message_id":1}

data: {"type":"content","content":"你好"}

data: {"type":"content","content":"！"}

data: {"type":"done","session_id":"...","message_id":2}
```

## 数据存储

SimpleChat 使用 SQLite 存储会话和消息。

默认数据库路径：

```txt
data/simplechat.db
```

数据库只存两类会变化的数据：

| 表 | 用途 |
| ---- | ---- |
| `sessions` | 会话列表 |
| `messages` | 聊天消息 |

不会放进数据库的内容：

- 用户和密码：放在 `config/settings.json`
- API Key：放在 `config/models.json`
- 模型选择偏好：放在浏览器 localStorage
- 主题设置：放在浏览器 localStorage
- 系统提示词：放在 `config/settings.json`

## 技术栈

- [Go](https://go.dev)
- [Gin](https://gin-gonic.com)
- [SQLite](https://www.sqlite.org)
- [mattn/go-sqlite3](https://github.com/mattn/go-sqlite3)
- [golang-jwt/jwt](https://github.com/golang-jwt/jwt)
- [Marked.js](https://github.com/markedjs/marked)
- [DOMPurify](https://github.com/cure53/DOMPurify)
- [highlight.js](https://highlightjs.org)

## Makefile 命令

| 命令 | 说明 |
| ---- | ---- |
| `make deps` | 下载前端依赖到 `backend/frontend/libs/` |
| `make build` | 下载依赖并构建二进制 |
| `make run` | 构建并运行 |
| `make docker` | 构建 Docker 镜像 |
| `make up` | 使用 Docker Compose 后台启动 |
| `make down` | 停止 Docker Compose 服务 |
| `make clean` | 删除二进制和前端依赖 |

## 开发与验证

```sh
make clean
make build
```

也可以直接在后端目录验证：

```sh
cd backend
go build ./...
```

Docker 验证：

```sh
docker compose up --build
```

## 常见问题

### 首次运行后服务退出，提示需要配置 API Key

这是正常的。

首次运行会生成默认配置文件，其中 `models.json` 的 `api_key` 是占位值：

```json
"api_key": "sk-your-api-key-here"
```

请改成真实 API Key 后重启服务。

如果你使用本地模型且不需要密钥，可以设为空字符串：

```json
"api_key": ""
```

### Docker 生成的 config/data 目录无法编辑

可以先在宿主机手动创建目录：

```sh
mkdir -p config data
```

如果已经生成且权限不对，可以执行：

```sh
sudo chown -R $USER:$USER config data
```

### 修改配置后为什么没有生效？

配置只在启动时加载一次。  
修改 `config/settings.json` 或 `config/models.json` 后，需要重启服务。

## License

MIT License - see [LICENSE](https://github.com/BaoPaper/SimpleChat/blob/main/LICENSE)

<!-- links -->

[your-project-path]: BaoPaper/SimpleChat
[contributors-shield]: https://img.shields.io/github/contributors/BaoPaper/SimpleChat.svg?style=flat-square
[contributors-url]: https://github.com/BaoPaper/SimpleChat/graphs/contributors
[forks-shield]: https://img.shields.io/github/forks/BaoPaper/SimpleChat.svg?style=flat-square
[forks-url]: https://github.com/BaoPaper/SimpleChat/network/members
[stars-shield]: https://img.shields.io/github/stars/BaoPaper/SimpleChat.svg?style=flat-square
[stars-url]: https://github.com/BaoPaper/SimpleChat/stargazers
[issues-shield]: https://img.shields.io/github/issues/BaoPaper/SimpleChat.svg?style=flat-square
[issues-url]: https://github.com/BaoPaper/SimpleChat/issues
[license-shield]: https://img.shields.io/github/license/BaoPaper/SimpleChat.svg?style=flat-square
[license-url]: https://github.com/BaoPaper/SimpleChat/blob/main/LICENSE
```
