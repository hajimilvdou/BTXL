# BTXL

[English](README.md) | 中文

BTXL 是一个基于 [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) 二次开发的开源社区额度平台。它将偏单用户、本地化的 CLI 代理扩展为适合运营场景的多用户服务，提供额度分发、共享凭证池、风控审计以及可视化面板。

## 项目概述

BTXL 面向的是“对外提供共享 AI 访问能力”的场景，而不是单纯的个人代理工具。

典型使用流程如下：

1. 管理员配置上游凭证、路由规则和安全策略。
2. 用户注册账号，并通过邀请码或兑换码领取额度。
3. 请求以 OpenAI、Claude 或 Gemini 兼容协议进入 BTXL。
4. BTXL 根据额度、风控与路由策略选择合适的上游凭证并转发请求。
5. 管理员通过 Web 面板管理用户、凭证、额度和系统设置。

## 核心能力

- 多用户账户体系与 JWT 鉴权
- 共享凭证池与模型路由
- 邀请、返佣、兑换码工作流
- 额度配置与策略控制
- IP 控制、异常检测、审计日志等风控能力
- 内嵌用户端与管理端可视化面板

## 技术栈

| 层级 | 技术 |
| --- | --- |
| 后端 | Go 1.26、Gin |
| 数据库 | 默认 SQLite（`modernc.org/sqlite`，无 CGO） |
| 前端 | React 19、TypeScript、Vite、Tailwind CSS |
| 认证 | JWT + 各厂商 OAuth 流程 |
| 容器 | Docker 多架构镜像 |
| 交付 | GitHub Actions + GHCR |

## 端口与访问模型

### 运行端口

| 端口 | 是否必须 | 作用 |
| --- | --- | --- |
| `8317` | 是 | 主 HTTP 服务端口，同时承载 API 网关、Web 面板与管理接口。 |

### 访问路径

BTXL 的 API 与面板使用同一个运行端口。

| 功能 | 路径 |
| --- | --- |
| 根路径 | `/` |
| Web 面板 | `/panel/` |
| OpenAI 兼容 API | `/v1/...` |
| Gemini 兼容 API | `/v1beta/...` |
| 管理 API | `/v0/management/...` |

当前行为如下：

- 当社区面板启用时，浏览器访问 `/` 会自动跳转到 `/panel/`。
- 非浏览器客户端访问 `/` 时，仍然返回 JSON 根响应。
- 不需要额外再开放单独的面板端口。

### OAuth 回调辅助端口

下面这些端口出现在部分厂商登录流程中，但它们不是标准公网服务端口，不应在常规部署中暴露：

| 端口 | 作用 |
| --- | --- |
| `8085` | Gemini 本地回调 |
| `1455` | Codex / OpenAI OAuth 本地回调 |
| `54545` | Claude 本地回调 |
| `51121` | Antigravity 本地回调 |
| `11451` | iFlow 本地回调 |

## 部署

### Docker Compose

主 `docker-compose.yml` 面向部署场景，设计目标是“复制即用”。

```bash
git clone https://github.com/hajimilvdou/BTXL.git
cd BTXL
docker compose up -d
```

首次启动后，容器会自动初始化以下宿主机目录：

- `./data/config/config.yaml`
- `./data/auths/`
- `./data/logs/`

### 部署模型

| 项目 | 值 |
| --- | --- |
| 服务名 | `btxl` |
| 容器名 | `btxl` |
| 发布端口 | `${BTXL_PORT:-8317}:8317` |
| 配置挂载 | `${BTXL_CONFIG_DIR:-./data/config}` -> `/opt/btxl/config` |
| 认证目录挂载 | `${BTXL_AUTH_PATH:-./data/auths}` -> `/root/.btxl` |
| 日志目录挂载 | `${BTXL_LOG_PATH:-./data/logs}` -> `/opt/btxl/logs` |
| 健康检查 | `http://127.0.0.1:8317/` |

### 直接拉镜像运行

镜像地址：

```bash
ghcr.io/hajimilvdou/btxl:latest
```

镜像同时兼容“文件式挂载”和“目录式挂载”。如果运行时没有检测到配置文件，会根据 `config.example.yaml` 自动初始化 `config/config.yaml`。

示例命令：

```bash
docker run -d \
  --name btxl \
  -p 8317:8317 \
  -v $(pwd)/data/config:/opt/btxl/config \
  -v $(pwd)/data/auths:/root/.btxl \
  -v $(pwd)/data/logs:/opt/btxl/logs \
  ghcr.io/hajimilvdou/btxl:latest
```

准备示例：

```bash
mkdir -p data/config data/auths data/logs
cp config.example.yaml data/config/config.yaml
```

### 环境变量

| 变量名 | 默认值 | 作用 |
| --- | --- | --- |
| `BTXL_IMAGE` | `ghcr.io/hajimilvdou/btxl:latest` | `docker compose` 使用的镜像地址 |
| `BTXL_PORT` | `8317` | 宿主机发布端口 |
| `BTXL_CONFIG_DIR` | `./data/config` | 宿主机配置目录 |
| `BTXL_AUTH_PATH` | `./data/auths` | 宿主机认证目录 |
| `BTXL_LOG_PATH` | `./data/logs` | 宿主机日志目录 |

### 本地源码构建

本地构建镜像时，使用 `docker-compose.dev.yml` 作为覆盖层：

```bash
docker compose -f docker-compose.yml -f docker-compose.dev.yml build
docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d
```

## 配置说明

### `config.example.yaml`

示例配置可以直接启动，但在生产环境中仍应逐项检查。

重点字段：

- `port`：主运行端口，默认 `8317`
- `api-keys`：客户端访问 API 时使用的密钥
- `auth-dir`：上游凭证存储目录
- `remote-management.secret-key`：管理 API 的密钥
- `community.*`：社区平台与面板相关配置

### 社区面板

内嵌面板由 `community` 配置段控制。

```yaml
community:
  enabled: true
  panel:
    enabled: true
    base-path: "/panel"
```

使用当前镜像初始化的新部署，默认已经启用社区面板。

运行时默认值也会在 `community` 配置段缺失时自动启用社区面板；只有当配置中显式写了 `community.enabled: false` 或 `community.panel.enabled: false` 时，`/panel/` 才会不可用。

### 安全提示

`config.example.yaml` 中的 `jwt-secret` 是占位值，生产环境必须替换。

## 运维说明

- API 与面板共用同一端口。
- 公网部署通常只需要开放 `8317`。
- 管理 API 仍保留在 `/v0/management/...`。
- OAuth 回调辅助端口不应作为公网服务端口开放。

## 常见问题

### 访问根路径显示 JSON，而不是面板

请检查实际运行配置 `data/config/config.yaml`。

常见原因：

- `community.enabled` 不存在或为 `false`
- `community.panel.enabled` 不存在或为 `false`
- 当前容器仍在使用较早生成的旧配置文件

### `/panel/` 返回 `404`

确认配置文件包含以下内容：

```yaml
community:
  enabled: true
  panel:
    enabled: true
    base-path: "/panel"
```

### Docker 报错 `not a directory`，提示 `config.yaml` 挂载失败

部分面板会把缺失的宿主机 `config.yaml` 路径自动创建成目录。BTXL 当前采用目录挂载来规避该问题：

- 宿主机：`./data/config`
- 容器内：`/opt/btxl/config`
- 实际配置文件：`/opt/btxl/config/config.yaml`

### Web 面板中的厂商 OAuth 流程无法完成

这通常不是 Docker 端口映射问题。相关流程依赖 localhost 回调语义，与主运行端口是两套逻辑。

## 项目结构

```text
cmd/server/              启动入口
internal/community/      社区平台核心
internal/panel/web/      React 前端
internal/api/            HTTP 服务与管理接口
docs/                    项目文档
test/                    集成与回归测试
```

## 致谢

本项目基于 [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI)（Router-For.ME）二次开发。

## License

MIT，详见 `LICENSE`。
