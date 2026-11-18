# ECS CDT 管理面板

轻量级、低内存占用的阿里云 CDT 流量监控与 ECS 自动化管理面板（Go 单二进制 + 内嵌静态前端 + SQLite）。

## 功能特性

- **极低资源占用**：Go 单二进制 + 内嵌前端 + SQLite，Alpine 镜像仅约 10 MB，容器内实测常驻内存 < 15 MB，可直接限制 `mem_limit: 64m`。
- **多阿里云账号管理**：
  - 在「账号」弹窗维护多个不同账户下的阿里云 AccessKey，支持连通性测试；添加实例选择账号时自动拉取可用 ECS 实例，未获取到时支持手动录入。
  - 系统按账号独立统计 CDT 境内/境外月度流量，并在添加/更新账号或实例后立即执行一次巡检。
- **CDT 流量监控与熔断**：
  - 自动区分境内（`cn-*` 排除 `cn-hongkong`、`cn-taiwan`）与境外流量，展示分地域明细，支持超限自动关机熔断。
- **ECS 定时控制与手动防护**：
  - 支持跨零点定时开关机窗口（如 `08:00–00:00`、`22:00–06:00`）。
  - 支持「自动开关机 / 仅超限关机 / 仅监控流量」三种策略模式，默认采用 `StopCharging`（节省停机）计费模式（可按实例切换为 `KeepCharging`）。
  - 针对实例缺失或过渡态（`Starting` / `Stopping`）自动跳过指令，防止无效 API 调用刷屏。
- **单页简洁交互**：
  - 账号凭证、实例配置、通知与调度设置全部弹窗完成；实例详情页内嵌实时巡检心跳与持久化审计日志。
- **通知推送**：支持 Bark 与通用 Webhook（飞书、钉钉、自定义接口），支持按事件订阅。
- **安全鉴权**：账号密码通过容器环境变量配置，内置防爆破限流、HttpOnly Cookie 会话及 SQLite AES-GCM 凭证加密存储。

## 快速部署（Docker Compose）

1. 复制环境变量模板并修改密码：

```bash
cp .env.example .env
# 编辑 .env 中的 PANEL_PASSWORD
```

2. 启动容器（使用 GitHub Container Registry 预构建多架构镜像，或添加 `--build` 本地构建）：

```bash
docker compose up -d
# 若需从本地源码构建：
docker compose up -d --build
```

3. 浏览器访问 `http://<服务器IP>:8080`，使用配置的 `PANEL_USERNAME` / `PANEL_PASSWORD` 登录。

## 环境变量说明

| 变量名 | 默认值 | 说明 |
| :--- | :--- | :--- |
| `PANEL_USERNAME` | `admin` | 面板登录用户名 |
| `PANEL_PASSWORD` | 无（未填时生成随机密码输出到日志） | 面板登录密码 |
| `PANEL_PASSWORD_HASH` | 无 | 可选 bcrypt 哈希密码（优先级高于 `PANEL_PASSWORD`） |
| `SESSION_SECRET` | 自动生成 | 会话签名与凭证加密主密钥（自动保存于 `/app/data/.session_secret`） |
| `PORT` | `8080` | Web 服务监听端口 |
| `TZ` | `Asia/Shanghai` | 容器时区（决定定时开关机时间） |
| `DATA_DIR` | `/app/data` | SQLite 数据库持久化目录 |
| `CHECK_INTERVAL` | `60` | 默认巡检间隔（秒） |
| `COOKIE_SECURE` | `false` | 启用 HTTPS 反代时建议设为 `true` |

## 本地开发运行

```bash
PANEL_USERNAME=admin PANEL_PASSWORD=devpass go run .
```

## 测试

```bash
go test ./...
```
