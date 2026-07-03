# SkillHub 基础骨架（子项目 A）设计

状态：草案
日期：2026-07-03
范围：仅子项目 A（基础骨架）。认证、领域模型、业务路由、i18n、真实迁移均不在本范围内，归后续子项目 B–H。

## 1. 背景与目标

SkillHub 是一个企业级 skill 包管理平台。整体被分解为 8 个子项目（A–H），各自走独立的 spec → plan → 实现循环。本子项目 A 是所有后续子项目的前提：定下项目结构、配置、日志、数据库、Redis、可插拔存储抽象与 HTTP 服务器骨架。

**目标**
- 提供可运行的 Go 服务，暴露 `/healthz`。
- 定下全局工程约定（目录、配置、日志、错误模型、存储接口），后续子项目直接复用。
- 提供本地开发一键编排（PG + Redis + MinIO）。

**非目标**
- 认证、用户、团队、技能包、搜索、社交、治理、i18n 的任何业务逻辑。
- 除占位外的数据库迁移。
- 任何前端模板/页面。

## 2. 技术选型

| 关注点 | 选型 |
|--------|------|
| 语言 | Go（最新稳定版） |
| Web 框架 | Gin |
| ORM | GORM（pgx 驱动） |
| 日志 | zap |
| 配置 | viper（YAML + 环境变量覆盖） |
| 迁移 | golang-migrate（版本化 SQL，up/down） |
| Redis | go-redis v9 |
| 对象存储 SDK | minio-go（兼容 S3/MinIO） |
| 本地编排 | docker-compose + Makefile |

## 3. 项目布局

```
skillhub/
  cmd/
    skillhub/main.go        # 入口：config → log → db/redis/storage → gin → Serve
    migrate/main.go         # golang-migrate 包装：up/down/create
  internal/
    config/                 # 配置加载与校验
      config.go
    log/                    # zap 初始化
      log.go
    db/                     # GORM 连接池
      db.go
    redis/                  # go-redis 客户端
      redis.go
    storage/
      storage.go            # Store 接口 + ObjectInfo
      local.go              # 文件系统实现
      s3.go                 # S3/MinIO 实现
    httpserver/
      server.go             # gin engine 装配 + /healthz
      middleware/
        recover.go
        requestid.go
        accesslog.go
        errors.go
    apperr/                 # 应用错误模型 + HTTP 映射
      apperr.go
  migrations/               # 版本化 SQL（含一个占位迁移）
  config/config.yaml        # 默认配置
  deployments/docker-compose.yml   # PG + Redis + MinIO
  Makefile
  go.mod
  README.md
```

## 4. 组件设计

### 4.1 config
- 用 viper 读 `config/config.yaml`，环境变量 `SKILLHUB_<SECTION>_<KEY>` 覆盖（例：`SKILLHUB_DB_HOST`）。
- 反序列化到强类型 struct，启动时校验必填项，缺失则 fail-fast。
- 分区：`server`（addr、read_timeout、write_timeout）、`db`（host、port、name、user、password、sslmode、max_open、max_idle）、`redis`（addr、password、db）、`storage`（driver、local.root、s3.endpoint、s3.bucket、s3.region、s3.access_key、s3.secret_key）、`log`（level、format）。

### 4.2 log
- zap 实例，level/format 来自配置。
- 暴露 `New(cfg) -> *zap.Logger`。
- 请求 id 由中间件注入到日志上下文。

### 4.3 db
- GORM + pgx 驱动，连接池参数来自配置。
- 启动时 ping 校验连通性，失败 fail-fast。
- 暴露 `New(cfg) -> *gorm.DB`。

### 4.4 redis
- go-redis v9，暴露 `New(cfg) -> *redis.Client`，启动 ping。

### 4.5 storage（可插拔）
接口：
```go
type ObjectInfo struct {
    Key          string
    Size         int64
    ContentType  string
    LastModified time.Time
}

type Store interface {
    Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) (location string, err error)
    Get(ctx context.Context, key string) (io.ReadCloser, error)
    Delete(ctx context.Context, key string) error
    Stat(ctx context.Context, key string) (ObjectInfo, error)
}
```
- `local` 实现：在配置的 `storage.local.root` 目录下按 key 存文件。key 含 `/` 时映射为子目录。
- `s3` 实现：minio-go 客户端，PutObject/GetObject/RemoveObject/StatObject。
- 工厂函数按 `storage.driver` 返回对应实现，未知值 fail-fast。

### 4.6 httpserver
- 装配 gin engine，注册中间件顺序：recover → requestid → accesslog → errors。
- `/healthz`：检查 db.Ping 与 redis.Ping，返回 JSON `{ "status": "ok"|"degraded", "checks": {...} }`，对应 HTTP 200/503。
- 优雅关闭：捕获 SIGTERM/SIGINT，Shutdown 上下文超时来自配置。
- 暴露 `Run(srv *http.Server)`。

### 4.7 middleware
- **recover**：捕获 panic，记日志，返回 500。
- **requestid**：从 `X-Request-Id` 取或生成 UUID，写入 context 与响应头。
- **accesslog**：记录 method/path/status/latency/request_id。
- **errors**：统一把 `apperr.Error` 映射为 JSON 响应 `{ "error": { "code", "message", "request_id" } }`。

### 4.8 apperr
- 类型 `Error { Code string; Category string; Message string; Cause error }`，实现 `Error()`/`Unwrap()`。
- `Code` 是稳定机器码（如 `db_ping_failed`），`Category` 用于日志分组。
- 函数 `HTTPStatus(err) int` 做映射，未识别默认 500。

### 4.9 migrate 命令
- 包装 golang-migrate，子命令 `up` / `down` / `create <name>`。
- 迁移文件放 `migrations/`，命名 `NNNNNN_name.up.sql` / `.down.sql`。
- 起步含一个占位迁移（建一个 `schema_meta` 表或空操作），验证管线可用。

## 5. 配置示例（config/config.yaml）

```yaml
server:
  addr: ":8080"
  read_timeout: 15s
  write_timeout: 15s
  shutdown_timeout: 10s
db:
  host: localhost
  port: 5432
  name: skillhub
  user: skillhub
  password: ""
  sslmode: disable
  max_open: 25
  max_idle: 5
redis:
  addr: localhost:6379
  password: ""
  db: 0
storage:
  driver: local          # local | s3
  local:
    root: ./var/storage
  s3:
    endpoint: localhost:9000
    bucket: skillhub
    region: us-east-1
    access_key: ""
    secret_key: ""
    use_ssl: false
log:
  level: info            # debug | info | warn | error
  format: json           # json | console
```

## 6. 本地开发编排

`deployments/docker-compose.yml` 起三个服务：
- Postgres（暴露 5432，带健康检查）
- Redis（暴露 6379）
- MinIO（暴露 9000 API + 9001 控制台，启动时自动创建 `skillhub` bucket）

`Makefile` 目标：
- `make run` — 起服务
- `make migrate-up` / `make migrate-down` / `make migrate-create NAME=...`
- `make test` — go test ./...
- `make lint` — golangci-lint
- `make compose-up` / `make compose-down`

## 7. 数据流

启动：`main → config.Load → log.New → db.New / redis.New / storage.New → httpserver.New → httpserver.Run`。
请求：`HTTP → recover → requestid → accesslog → errors → handler → response`。
关闭：`SIGTERM → http.Server.Shutdown(ctx) → 关闭 db/redis 连接`。

## 8. 错误处理

- 所有组件构造函数失败均返回 `*apperr.Error`，main 捕获后记 fatal 日志退出。
- handler 返回 `*apperr.Error` 由 errors 中间件统一渲染。
- 配置缺失、驱动未知、依赖 ping 失败均为 fail-fast。

## 9. 测试策略

- **单元测试**：config 解析、apperr 映射、storage local 实现（用 tempdir，无外部依赖）。
- **集成测试**：storage s3 实现用 compose 里的 MinIO；db/redis 用 compose 起的实例。用 build tag `integration` 隔离，`make test` 默认只跑单元测试，`make test-integration` 跑集成。
- **烟雾测试**：`/healthz` 返回 200。

## 10. 交付物

- 可 `make compose-up && make migrate-up && make run` 起来的服务，`GET /healthz` 返回 200。
- 完整的工程约定（目录、配置、日志、错误、存储接口），后续子项目在此基础上增量开发。

## 11. 后续子项目衔接

- B 认证：复用 db/redis/session，加用户表与中间件。
- C 团队：复用 db、RBAC 基础。
- D 发布：复用 storage.Store 存包，db 存元数据。
- E 发现：复用 db 的 tsvector。
- F 治理：复用审计表（B 阶段建）。
- G 社交：复用 db。
- H i18n：在 httpserver 加 go-i18n bundle 中间件（骨架阶段不实现）。
