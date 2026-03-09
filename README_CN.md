# BTXL

[English](README.md) | 中文

BTXL（冰糖雪梨）是一个基于 [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) 二次开发的开源社区额度平台。它将原本偏单用户、本地化的 CLI 代理，扩展为支持用户注册、额度分发、共享凭证池调度、风控审计以及 Web 面板管理的多用户服务。

## 项目定位

BTXL 面向的是“运营一个共享 AI 访问平台”的场景，而不只是“自己本地跑一个代理”。

典型链路如下：

1. 管理员配置上游凭证、模型路由和安全策略。
2. 用户注册账号，通过邀请码 / 兑换码领取额度。
3. 请求以 OpenAI / Claude / Gemini 兼容方式进入 BTXL。
4. BTXL 根据路由、额度和风控策略选择上游凭证并转发。
5. 管理员通过 Web 面板管理用户、凭证、额度和系统配置。

## 相比上游 CLIProxyAPI 的增强

- 多用户社区平台能力
- 额度模板、兑换码、邀请 / 返佣流程
- 共享凭证池与调度策略
- 用户端 / 管理端 Web 面板
- IP 控制、限流、异常检测、审计日志
- 面向社区运营，而不仅是本地单用户代理

## 技术栈

| 层级 | 技术 |
| --- | --- |
| 后端 | Go 1.26、Gin |
| 数据库 | 默认 SQLite（`modernc.org/sqlite`，无 CGO），部分场景支持 PostgreSQL |
| 前端 | React 19、TypeScript、Vite、Tailwind CSS、Zustand、Recharts |
| 认证 | JWT + 各厂商 OAuth 流程 |
| 容器 | Docker 多架构镜像 |
| CI/CD | GitHub Actions + GHCR |

## 端口说明

这是本仓库部署时最容易混淆、也最关键的一部分。

### 服务器真正需要开放的端口

| 端口 | 是否必须 | 作用 |
| --- | --- | --- |
| `8317` | 是 | 主 HTTP 服务端口。承载代理 API；若配置开启社区模块，也会承载 Web 面板和相关管理路由。 |

### OAuth 回调辅助端口

下面这些端口虽然在代码中存在，但它们**不是常规服务器运行端口**，也**不应该**在标准 Docker 服务器部署中对外暴露。

| 端口 | 对应流程 | 作用 |
| --- | --- | --- |
| `8085` | Gemini | Gemini OAuth 登录时使用的 localhost 回调端口 |
| `1455` | Codex / OpenAI OAuth | Codex / OpenAI OAuth 登录时使用的 localhost 回调端口 |
| `54545` | Claude | Claude OAuth 登录时使用的 localhost 回调端口 |
| `51121` | Antigravity | Antigravity OAuth 登录时使用的 localhost 回调端口 |
| `11451` | iFlow | iFlow OAuth 登录时使用的 localhost 回调端口 |

请注意：

- **容器化服务器部署时，只需要发布 `8317`。**
- 把这些回调端口映射到公网，**并不能**让远程 Web 面板 OAuth 自动恢复正常。
- 如果你的部署面板 / PaaS 会根据镜像自动识别并开放端口，它理论上只需要处理 `8317`。

## 为什么之前的部署看起来“不对”

旧的 `docker-compose.yml` 同时暴露了多个 OAuth 回调端口，这在服务器部署场景里会造成误导，原因有三点：

- 应用作为常规 HTTP 服务，真正长期监听的只有主端口（默认 `8317`）；
- 额外端口属于特定 OAuth 辅助回调流程，不是公网服务端口；
- 很多服务器部署平台只会关注镜像的真实服务端口，错误地把回调端口当成“必须开放端口”会让排障方向跑偏。

现在仓库已将 Docker 运行语义统一为：**标准服务端口只有 `8317`。**

## Docker 部署

### Docker Compose

当前 compose 的服务名和容器名都已经改为 `btxl`。

```bash
git clone https://github.com/hajimilvdou/BTXL.git
cd BTXL

docker compose up -d
```

首次启动后，容器会自动初始化：

- `./data/config/config.yaml`
- `./data/auths/`
- `./data/logs/`

之后你只需要修改 `./data/config/config.yaml`，再重启容器即可。

当前 compose 行为：

- 服务名：`btxl`
- 容器名：`btxl`
- 对外端口：`8317:8317`
- 容器内配置目录挂载路径：`/opt/btxl/config`
- 容器内认证目录挂载路径：`/root/.btxl`
- 容器内日志目录挂载路径：`/opt/btxl/logs`

### 直接拉镜像部署

镜像地址：

```bash
ghcr.io/hajimilvdou/btxl:latest
```

镜像现在使用了更稳健的启动脚本，同时兼容“文件挂载”和“目录挂载”两种配置方式；如果配置缺失，会自动根据 `config.example.yaml` 初始化。

这专门规避了面板 / PaaS 常见问题：宿主机上不存在 `config.yaml` 时，平台会自动创建成目录，随后 Docker 因“把目录挂到文件路径”而直接启动失败。

但正式部署时，仍然建议挂载自己的配置和持久化目录：

```bash
docker run -d \
  --name btxl \
  -p 8317:8317 \
  -v $(pwd)/data/config:/opt/btxl/config \
  -v $(pwd)/data/auths:/root/.btxl \
  -v $(pwd)/data/logs:/opt/btxl/logs \
  ghcr.io/hajimilvdou/btxl:latest
```

首次启动前建议先创建目录：

```bash
mkdir -p config auths logs
cp config.example.yaml config/config.yaml
```

### Docker 环境变量

| 变量名 | 默认值 | 作用 |
| --- | --- | --- |
| `BTXL_IMAGE` | `ghcr.io/hajimilvdou/btxl:latest` | `docker compose` 使用的镜像标签 |
| `BTXL_CONFIG_DIR` | `./data/config` | 宿主机配置目录挂载路径 |
| `BTXL_AUTH_PATH` | `./data/auths` | 宿主机认证目录，挂载到 `/root/.btxl` |
| `BTXL_LOG_PATH` | `./data/logs` | 宿主机日志目录挂载路径 |

兼容说明：

- 容器启动脚本仍兼容旧的 `/opt/btxl/config.yaml` 文件式挂载目标
- 这是为了兼容某些面板已经错误创建出 `config.yaml` 目录的情况，升级镜像后仍可继续启动

## 配置说明

### `config.example.yaml`

仓库自带的是“可启动模板”，但你仍然需要在生产环境中修改关键项。

重点关注：

- `port`：主服务端口，默认 `8317`
- `api-keys`：给客户端使用的 API Key
- `auth-dir`：上游凭证存放目录
- `remote-management.secret-key`：管理接口密钥
- `community.*`：BTXL 社区平台功能配置

### Web 面板为何可能访问不到

仓库里有前端代码，不代表 `/panel` 会默认生效。

旧的上游管理页已经从 BTXL 中移除。

若要启用 BTXL 的社区面板，需要在配置文件中显式开启社区模块，例如：

```yaml
community:
  enabled: true
  panel:
    enabled: true
    base-path: "/panel"
```

如果 `community.enabled` 没开，代理服务依然可能正常启动，但社区面板路由不会注册，因此访问 `/panel` 会得到 `404`。

BTXL 现在只保留新的社区面板 `/panel`。

## GitHub Actions 与镜像发布

仓库当前已经通过 GitHub Actions 自动发布镜像到 GHCR。

现有链路为：

1. `docker-image.yml` 构建 `amd64` / `arm64` 镜像；
2. 推送各架构镜像标签；
3. 生成并推送 multi-arch manifest；
4. 服务器端直接拉取 `ghcr.io/hajimilvdou/btxl:latest` 部署。

服务器部署建议：

- 只发布 `8317`；
- 正式环境挂载真实 `config.yaml`；
- 持久化 `auths` 和 `logs`；
- 不要把 OAuth 回调端口误当成公网服务端口。

## 本地构建

### 编译二进制

```bash
go build -o btxl ./cmd/server
./btxl
```

### Docker 辅助脚本

- `docker-build.sh`
- `docker-build.ps1`

这两个脚本现在统一使用本地镜像标签 `btxl:local`。

## 项目结构

```text
cmd/server/              启动入口
internal/community/      BTXL 社区平台核心
internal/panel/web/      React 前端
internal/api/            HTTP 服务与管理接口
auths/                   本地上游凭证目录
docs/                    项目文档
test/                    集成 / 回归测试
```

## 常见问题

### 容器启动了，但服务访问不到

建议按这个顺序排查：

1. 确认平台是否真的开放了 `8317`；
2. 确认容器使用的是有效的 `config/config.yaml`；
3. 确认当前不是“云部署模式等待配置”状态；
4. 确认挂载进去的 YAML 没有语法错误，且 `port` 配置合法。

### Docker 报错 `not a directory`，提示挂载 `config.yaml` 失败

这通常说明：你的面板把宿主机上的 `config.yaml` 自动创建成了目录，但容器却把它当成文件来挂载。

BTXL 现在的推荐方式已经改成目录挂载：

- 宿主机：`./data/config`
- 容器内：`/opt/btxl/config`
- 实际配置文件：`/opt/btxl/config/config.yaml`

如果你的面板上已经存在一个名为 `config.yaml` 的目录，升级到新镜像后通常也能兼容启动，因为入口脚本会自动识别这种情况，并尝试使用里面的 `config.yaml/config.yaml`。

### 服务器部署后，Web 面板里某些 OAuth 登录流程无法完成

这通常**不是**简单的 Docker 端口映射问题。

因为相关流程依赖 `localhost` 回调语义，所以把 `8085`、`1455`、`54545`、`51121`、`11451` 暴露到公网，通常也不是正确修复方式。

### `/panel` 访问是 404

检查 `config.yaml` 中是否启用了 `community.enabled` 与 `community.panel.enabled`。

## 致谢

本项目基于 [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI)（Router-For.ME）二次开发。

## License

MIT，详见 `LICENSE`。
