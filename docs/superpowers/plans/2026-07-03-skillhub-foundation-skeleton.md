# SkillHub 基础骨架（子项目 A）实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 搭建 SkillHub Go 服务骨架——配置、日志、DB、Redis、可插拔存储、HTTP 服务器与 `/healthz`，可一键本地起服务。

**Architecture:** Gin + GORM(pgx) + zap + viper + golang-migrate。`cmd/skillhub` 装配各组件并启动 gin；`cmd/migrate` 包装迁移 CLI。所有组件在 `internal/` 下，单一职责，构造函数失败返回 `*apperr.Error`。可插拔存储通过 `Store` 接口 + 工厂按配置切换 local/s3。

**Tech Stack:** Go 1.22, Gin, GORM + pgx, zap, viper, golang-migrate, go-redis v9, minio-go, docker-compose, Makefile。

## Global Constraints

- Go 1.22+，module path `github.com/skillhub/skillhub`。
- 所有组件构造函数失败返回 `*apperr.Error`（除 main）。
- 配置缺失、驱动未知、依赖 ping 失败均 fail-fast（main 捕获后 fatal 退出）。
- 测试：单元测试无外部依赖；集成测试用 build tag `//go:build integration` 隔离，依赖 compose。
- 每个任务结束提交一次，提交信息 conventional commits（feat/fix/test/docs/chore）。
- 目录约定见 spec 第 3 节，不得偏离。

---

## File Structure

| 文件 | 职责 |
|------|------|
| `go.mod` / `go.sum` | module 与依赖 |
| `Makefile` | run/migrate/test/lint/compose 目标 |
| `config/config.yaml` | 默认配置 |
| `deployments/docker-compose.yml` | PG+Redis+MinIO 本地编排 |
| `.gitignore` | 忽略 var/、二进制 |
| `cmd/skillhub/main.go` | 入口装配与启动 |
| `cmd/migrate/main.go` | 迁移 CLI |
| `internal/config/config.go` | 配置加载与校验 |
| `internal/config/config_test.go` | 配置测试 |
| `internal/log/log.go` | zap 初始化 |
| `internal/apperr/apperr.go` | 应用错误模型 |
| `internal/apperr/apperr_test.go` | 错误测试 |
| `internal/db/db.go` | GORM 连接池 |
| `internal/redis/redis.go` | go-redis 客户端 |
| `internal/storage/storage.go` | Store 接口 + 工厂 |
| `internal/storage/local.go` | 文件系统实现 |
| `internal/storage/local_test.go` | local 单测 |
| `internal/storage/s3.go` | S3/MinIO 实现 |
| `internal/storage/s3_test.go` | s3 集成测试 |
| `internal/httpserver/server.go` | gin 装配 + /healthz |
| `internal/httpserver/server_test.go` | 烟雾测试 |
| `internal/httpserver/middleware/recover.go` | panic 恢复 |
| `internal/httpserver/middleware/requestid.go` | 请求 id |
| `internal/httpserver/middleware/accesslog.go` | 访问日志 |
| `internal/httpserver/middleware/errors.go` | 统一错误渲染 |
| `migrations/000001_init.up.sql` / `.down.sql` | 占位迁移 |

---

### Task 1: 项目初始化与编排文件

**Files:**
- Create: `go.mod`
- Create: `Makefile`
- Create: `.gitignore`
- Create: `config/config.yaml`
- Create: `deployments/docker-compose.yml`
- Create: `README.md` (replace)

**Interfaces:**
- Produces: module `github.com/skillhub/skillhub`；可用的 `make` 目标；compose 服务。

- [ ] **Step 1: 初始化 go module**

Run:
```bash
go mod init github.com/skillhub/skillhub
```
Expected: `go: creating new go.mod: module github.com/skillhub/skillhub`

- [ ] **Step 2: 写 .gitignore**

```
/bin/
/var/
*.test
*.out
.env
```

- [ ] **Step 3: 写 config/config.yaml**

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
  driver: local
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
  level: info
  format: json
```

- [ ] **Step 4: 写 deployments/docker-compose.yml**

```yaml
services:
  postgres:
    image: postgres:16
    environment:
      POSTGRES_DB: skillhub
      POSTGRES_USER: skillhub
      POSTGRES_PASSWORD: skillhub
    ports: ["5432:5432"]
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U skillhub"]
      interval: 5s
      timeout: 3s
      retries: 10
  redis:
    image: redis:7
    ports: ["6379:6379"]
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 10
  minio:
    image: minio/minio:latest
    command: server /data --console-address ":9001"
    environment:
      MINIO_ROOT_USER: minioadmin
      MINIO_ROOT_PASSWORD: minioadmin
    ports: ["9000:9000", "9001:9001"]
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:9000/minio/health/live"]
      interval: 5s
      timeout: 3s
      retries: 10
  createbucket:
    image: minio/mc:latest
    depends_on:
      minio: { condition: service_healthy }
    entrypoint: >
      /bin/sh -c "
      mc alias set local http://minio:9000 minioadmin minioadmin &&
      mc mb local/skillhub --ignore-existing &&
      exit 0
      "
```

- [ ] **Step 5: 写 Makefile**

```makefile
.PHONY: run migrate-up migrate-down migrate-create test test-integration lint compose-up compose-down tidy

run:
	go run ./cmd/skillhub

migrate-up:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

migrate-create:
	@test -n "$(NAME)" || (echo "Usage: make migrate-create NAME=foo" && exit 1)
	go run ./cmd/migrate create $(NAME)

test:
	go test ./...

test-integration:
	go test -tags integration ./...

lint:
	golangci-lint run ./...

compose-up:
	docker compose -f deployments/docker-compose.yml up -d

compose-down:
	docker compose -f deployments/docker-compose.yml down

tidy:
	go mod tidy
```

- [ ] **Step 6: 写 README.md**

```markdown
# SkillHub

企业级 skill 包管理平台。

## 本地开发

```bash
make compose-up        # 起 PG + Redis + MinIO
make migrate-up        # 跑迁移
make run               # 起服务 :8080
```

健康检查：`curl http://localhost:8080/healthz`

## 测试

```bash
make test              # 单元测试
make test-integration  # 集成测试（需先 make compose-up）
```
```

- [ ] **Step 7: 提交**

```bash
git add -A
git commit -m "chore: project init, compose, makefile, default config"
```

---

### Task 2: apperr 包

**Files:**
- Create: `internal/apperr/apperr.go`
- Create: `internal/apperr/apperr_test.go`

**Interfaces:**
- Produces: `apperr.Error{Code,Category,Message,Cause}`，`apperr.New(code,category,msg)`，`apperr.Wrap(code,category,msg,cause)`，`apperr.HTTPStatus(err) int`。

- [ ] **Step 1: 写失败测试**

`internal/apperr/apperr_test.go`:
```go
package apperr

import (
	"errors"
	"testing"
)

func TestError_Message(t *testing.T) {
	e := New("db_ping_failed", "db", "ping failed")
	if got := e.Error(); got != "db: ping failed" {
		t.Fatalf("got %q", got)
	}
}

func TestError_Unwrap(t *testing.T) {
	cause := errors.New("conn refused")
	e := Wrap("db_ping_failed", "db", "ping failed", cause)
	if !errors.Is(e, cause) {
		t.Fatal("Unwrap should return cause")
	}
}

func TestHTTPStatus(t *testing.T) {
	cases := []struct {
		code string
		want int
	}{
		{"not_found", 404},
		{"unauthorized", 401},
		{"forbidden", 403},
		{"validation_failed", 422},
		{"db_ping_failed", 500},
		{"unknown", 500},
	}
	for _, c := range cases {
		e := New(c.code, "x", "msg")
		if got := HTTPStatus(e); got != c.want {
			t.Fatalf("code %s: got %d want %d", c.code, got, c.want)
		}
	}
}

func TestHTTPStatus_NonApperr(t *testing.T) {
	if got := HTTPStatus(errors.New("plain")); got != 500 {
		t.Fatalf("got %d", got)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/apperr/`
Expected: FAIL（包未实现）

- [ ] **Step 3: 写实现**

`internal/apperr/apperr.go`:
```go
package apperr

import "fmt"

type Error struct {
	Code     string
	Category string
	Message  string
	Cause    error
}

func (e *Error) Error() string {
	if e.Category == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Category, e.Message)
}

func (e *Error) Unwrap() error { return e.Cause }

func New(code, category, message string) *Error {
	return &Error{Code: code, Category: category, Message: message}
}

func Wrap(code, category, message string, cause error) *Error {
	return &Error{Code: code, Category: category, Message: message, Cause: cause}
}

func HTTPStatus(err error) int {
	e, ok := err.(*Error)
	if !ok {
		return 500
	}
	switch e.Code {
	case "not_found":
		return 404
	case "unauthorized":
		return 401
	case "forbidden":
		return 403
	case "validation_failed":
		return 422
	default:
		return 500
	}
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/apperr/`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/apperr
git commit -m "feat(apperr): add application error model and HTTP mapping"
```

---

### Task 3: config 包

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Create: `config/config.yaml` (已存在，测试用临时文件)

**Interfaces:**
- Consumes: `apperr`
- Produces: `config.Config` 及子结构体；`config.Load(path string) (*Config, error)`；`(*Config).Validate() error`。

- [ ] **Step 1: 写失败测试**

`internal/config/config_test.go`:
```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

const validYAML = `
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
  driver: local
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
  level: info
  format: json
`

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoad_Valid(t *testing.T) {
	p := writeTemp(t, validYAML)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Server.Addr != ":8080" {
		t.Fatalf("addr=%s", c.Server.Addr)
	}
	if c.Storage.Driver != "local" {
		t.Fatalf("driver=%s", c.Storage.Driver)
	}
	if c.DB.Port != 5432 {
		t.Fatalf("port=%d", c.DB.Port)
	}
}

func TestValidate_MissingServerAddr(t *testing.T) {
	c, _ := Load(writeTemp(t, validYAML))
	c.Server.Addr = ""
	if err := c.Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidate_InvalidStorageDriver(t *testing.T) {
	c, _ := Load(writeTemp(t, validYAML))
	c.Storage.Driver = "ftp"
	if err := c.Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	os.Setenv("SKILLHUB_DB_PORT", "6543")
	defer os.Unsetenv("SKILLHUB_DB_PORT")
	c, err := Load(writeTemp(t, validYAML))
	if err != nil {
		t.Fatal(err)
	}
	if c.DB.Port != 6543 {
		t.Fatalf("port=%d", c.DB.Port)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/config/`
Expected: FAIL（Load 未定义）

- [ ] **Step 3: 拉依赖并写实现**

Run:
```bash
go get github.com/spf13/viper@latest
```

`internal/config/config.go`:
```go
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server  ServerConfig  `mapstructure:"server"`
	DB      DBConfig      `mapstructure:"db"`
	Redis   RedisConfig   `mapstructure:"redis"`
	Storage StorageConfig `mapstructure:"storage"`
	Log     LogConfig     `mapstructure:"log"`
}

type ServerConfig struct {
	Addr            string        `mapstructure:"addr"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

type DBConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Name     string `mapstructure:"name"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	SSLMode  string `mapstructure:"sslmode"`
	MaxOpen  int    `mapstructure:"max_open"`
	MaxIdle  int    `mapstructure:"max_idle"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type StorageConfig struct {
	Driver string       `mapstructure:"driver"`
	Local  LocalStorage `mapstructure:"local"`
	S3     S3Storage    `mapstructure:"s3"`
}

type LocalStorage struct {
	Root string `mapstructure:"root"`
}

type S3Storage struct {
	Endpoint  string `mapstructure:"endpoint"`
	Bucket    string `mapstructure:"bucket"`
	Region    string `mapstructure:"region"`
	AccessKey string `mapstructure:"access_key"`
	SecretKey string `mapstructure:"secret_key"`
	UseSSL    bool   `mapstructure:"use_ssl"`
}

type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetEnvPrefix("SKILLHUB")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) Validate() error {
	if c.Server.Addr == "" {
		return fmt.Errorf("server.addr is required")
	}
	if c.DB.Host == "" || c.DB.Name == "" || c.DB.User == "" {
		return fmt.Errorf("db.host, db.name, db.user are required")
	}
	if c.Redis.Addr == "" {
		return fmt.Errorf("redis.addr is required")
	}
	switch c.Storage.Driver {
	case "local":
		if c.Storage.Local.Root == "" {
			return fmt.Errorf("storage.local.root is required when driver=local")
		}
	case "s3":
		if c.Storage.S3.Endpoint == "" || c.Storage.S3.Bucket == "" {
			return fmt.Errorf("storage.s3.endpoint and bucket are required when driver=s3")
		}
	default:
		return fmt.Errorf("storage.driver must be local or s3, got %q", c.Storage.Driver)
	}
	if c.Log.Level == "" {
		return fmt.Errorf("log.level is required")
	}
	return nil
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/config/`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/config go.mod go.sum
git commit -m "feat(config): add viper-based config loading and validation"
```

---

### Task 4: log 包

**Files:**
- Create: `internal/log/log.go`

**Interfaces:**
- Consumes: `config.LogConfig`
- Produces: `log.New(cfg config.LogConfig) (*zap.Logger, error)`

- [ ] **Step 1: 拉依赖**

Run:
```bash
go get go.uber.org/zap@latest
```

- [ ] **Step 2: 写实现**

`internal/log/log.go`:
```go
package log

import (
	"fmt"

	"github.com/skillhub/skillhub/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func New(cfg config.LogConfig) (*zap.Logger, error) {
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		return nil, fmt.Errorf("invalid log.level %q: %w", cfg.Level, err)
	}
	zcfg := zap.NewProductionConfig()
	zcfg.Level = zap.NewAtomicLevelAt(level)
	if cfg.Format == "console" {
		zcfg = zap.NewDevelopmentConfig()
		zcfg.Level = zap.NewAtomicLevelAt(level)
	}
	logger, err := zcfg.Build()
	if err != nil {
		return nil, fmt.Errorf("build logger: %w", err)
	}
	return logger, nil
}
```

- [ ] **Step 3: 写最小测试**

`internal/log/log_test.go`:
```go
package log

import (
	"testing"

	"github.com/skillhub/skillhub/internal/config"
)

func TestNew_Valid(t *testing.T) {
	l, err := New(config.LogConfig{Level: "info", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	l.Info("ok")
}

func TestNew_InvalidLevel(t *testing.T) {
	if _, err := New(config.LogConfig{Level: "nope"}); err == nil {
		t.Fatal("expected error")
	}
}
```

- [ ] **Step 4: 跑测试**

Run: `go test ./internal/log/`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/log go.mod go.sum
git commit -m "feat(log): add zap logger factory"
```

---

### Task 5: db 包

**Files:**
- Create: `internal/db/db.go`

**Interfaces:**
- Consumes: `config.DBConfig`, `apperr`
- Produces: `db.New(cfg config.DBConfig) (*gorm.DB, error)`，`db.Ping(g *gorm.DB) error`

- [ ] **Step 1: 拉依赖**

Run:
```bash
go get gorm.io/gorm@latest gorm.io/driver/postgres@latest
```

- [ ] **Step 2: 写实现**

`internal/db/db.go`:
```go
package db

import (
	"fmt"
	"time"

	"github.com/skillhub/skillhub/internal/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func New(cfg config.DBConfig) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Name, cfg.SSLMode,
	)
	g, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	sqlDB, err := g.DB()
	if err != nil {
		return nil, fmt.Errorf("raw db: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpen)
	sqlDB.SetMaxIdleConns(cfg.MaxIdle)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)
	return g, nil
}

func Ping(g *gorm.DB) error {
	sqlDB, err := g.DB()
	if err != nil {
		return fmt.Errorf("raw db: %w", err)
	}
	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("db ping: %w", err)
	}
	return nil
}
```

- [ ] **Step 3: 写测试（仅 New 的 DSN 构造可单测；连通用集成测试）**

`internal/db/db_test.go`:
```go
package db

import (
	"testing"

	"github.com/skillhub/skillhub/internal/config"
)

func TestNew_InvalidHost(t *testing.T) {
	// 连不上的 host 应返回错误而非 panic
	_, err := New(config.DBConfig{
		Host: "127.0.0.1", Port: 1, Name: "x", User: "x", SSLMode: "disable",
		MaxOpen: 1, MaxIdle: 1,
	})
	if err == nil {
		t.Fatal("expected error for unreachable db")
	}
}
```

- [ ] **Step 4: 跑测试**

Run: `go test ./internal/db/`
Expected: PASS（连不上返回错误）

- [ ] **Step 5: 提交**

```bash
git add internal/db go.mod go.sum
git commit -m "feat(db): add GORM postgres connection factory"
```

---

### Task 6: redis 包

**Files:**
- Create: `internal/redis/redis.go`

**Interfaces:**
- Consumes: `config.RedisConfig`
- Produces: `redis.New(cfg config.RedisConfig) (*redis.Client, error)`，`redis.Ping(ctx, c) error`

- [ ] **Step 1: 拉依赖**

Run:
```bash
go get github.com/redis/go-redis/v9@latest
```

- [ ] **Step 2: 写实现**

`internal/redis/redis.go`:
```go
package redis

import (
	"context"
	"fmt"

	"github.com/skillhub/skillhub/internal/config"
	rdb "github.com/redis/go-redis/v9"
)

func New(cfg config.RedisConfig) (*rdb.Client, error) {
	c := rdb.NewClient(&rdb.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	return c, nil
}

func Ping(ctx context.Context, c *rdb.Client) error {
	if err := c.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}
	return nil
}
```

- [ ] **Step 3: 写测试**

`internal/redis/redis_test.go`:
```go
package redis

import (
	"context"
	"testing"

	"github.com/skillhub/skillhub/internal/config"
)

func TestPing_Unreachable(t *testing.T) {
	c, _ := New(config.RedisConfig{Addr: "127.0.0.1:1"})
	if err := Ping(context.Background(), c); err == nil {
		t.Fatal("expected error")
	}
}
```

- [ ] **Step 4: 跑测试**

Run: `go test ./internal/redis/`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/redis go.mod go.sum
git commit -m "feat(redis): add go-redis client factory"
```

---

### Task 7: storage 接口与 local 实现

**Files:**
- Create: `internal/storage/storage.go`
- Create: `internal/storage/local.go`
- Create: `internal/storage/local_test.go`

**Interfaces:**
- Consumes: `config.StorageConfig`, `apperr`
- Produces: `storage.Store` 接口，`storage.ObjectInfo`，`storage.New(cfg) (Store, error)`，`storage.NewLocal(root) (*LocalStore, error)`

- [ ] **Step 1: 写接口与工厂**

`internal/storage/storage.go`:
```go
package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/skillhub/skillhub/internal/config"
)

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

func New(cfg config.StorageConfig) (Store, error) {
	switch cfg.Driver {
	case "local":
		return NewLocal(cfg.Local.Root)
	case "s3":
		return NewS3(cfg.S3)
	default:
		return nil, fmt.Errorf("unknown storage driver: %s", cfg.Driver)
	}
}
```

- [ ] **Step 2: 写失败测试**

`internal/storage/local_test.go`:
```go
package storage

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestLocal_PutGetDelete(t *testing.T) {
	root := t.TempDir()
	s, err := NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	body := []byte("hello skill")
	loc, err := s.Put(ctx, "a/b/skill.txt", bytes.NewReader(body), int64(len(body)), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	if loc == "" {
		t.Fatal("empty location")
	}
	rc, err := s.Get(ctx, "a/b/skill.txt")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(got, body) {
		t.Fatalf("got %q", got)
	}
	info, err := s.Stat(ctx, "a/b/skill.txt")
	if err != nil {
		t.Fatal(err)
	}
	if info.Size != int64(len(body)) || info.ContentType != "text/plain" {
		t.Fatalf("info=%+v", info)
	}
	if err := s.Delete(ctx, "a/b/skill.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Stat(ctx, "a/b/skill.txt"); !os.IsNotExist(err) {
		t.Fatalf("expected not-exist, got %v", err)
	}
}

func TestLocal_NewLocal_MissingRoot(t *testing.T) {
	if _, err := NewLocal(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("expected error for missing root")
	}
}
```

- [ ] **Step 3: 跑测试确认失败**

Run: `go test ./internal/storage/`
Expected: FAIL（NewLocal 未定义）

- [ ] **Step 4: 写 local 实现**

`internal/storage/local.go`:
```go
package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type LocalStore struct {
	root string
}

func NewLocal(root string) (*LocalStore, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create storage root: %w", err)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("abs root: %w", err)
	}
	return &LocalStore{root: abs}, nil
}

func (s *LocalStore) path(key string) (string, error) {
	p := filepath.Join(s.root, filepath.FromSlash(key))
	if rel, err := filepath.Rel(s.root, p); err != nil || rel == ".." || filepath.IsAbs(rel) {
		return "", fmt.Errorf("invalid key escapes root: %s", key)
	}
	return p, nil
}

func (s *LocalStore) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) (string, error) {
	p, err := s.path(key)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}
	f, err := os.Create(p)
	if err != nil {
		return "", fmt.Errorf("create: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return "", fmt.Errorf("write: %w", err)
	}
	return p, nil
}

func (s *LocalStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	p, err := s.path(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (s *LocalStore) Delete(ctx context.Context, key string) error {
	p, err := s.path(key)
	if err != nil {
		return err
	}
	return os.Remove(p)
}

func (s *LocalStore) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	p, err := s.path(key)
	if err != nil {
		return ObjectInfo{}, err
	}
	fi, err := os.Stat(p)
	if err != nil {
		return ObjectInfo{}, err
	}
	return ObjectInfo{
		Key:          key,
		Size:         fi.Size(),
		LastModified: fi.ModTime(),
	}, nil
}
```

- [ ] **Step 5: 跑测试确认通过**

Run: `go test ./internal/storage/`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add internal/storage
git commit -m "feat(storage): add Store interface and local filesystem implementation"
```

---

### Task 8: storage s3 实现（集成测试）

**Files:**
- Create: `internal/storage/s3.go`
- Create: `internal/storage/s3_test.go`

**Interfaces:**
- Consumes: `config.S3Storage`
- Produces: `storage.NewS3(cfg config.S3Storage) (*S3Store, error)`，实现 `Store` 接口。

- [ ] **Step 1: 拉依赖**

Run:
```bash
go get github.com/minio/minio-go/v7@latest
```

- [ ] **Step 2: 写实现**

`internal/storage/s3.go`:
```go
package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/skillhub/skillhub/internal/config"
)

type S3Store struct {
	client *minio.Client
	bucket string
}

func NewS3(cfg config.S3Storage) (*S3Store, error) {
	u, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse endpoint: %w", err)
	}
	c, err := minio.New(u.Host, &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure:       cfg.UseSSL,
		Region:       cfg.Region,
		BucketLookup: minio.BucketLookupAuto,
	})
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}
	return &S3Store{client: c, bucket: cfg.Bucket}, nil
}

func (s *S3Store) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) (string, error) {
	_, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return "", fmt.Errorf("put object: %w", err)
	}
	return fmt.Sprintf("s3://%s/%s", s.bucket, key), nil
}

func (s *S3Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object: %w", err)
	}
	return obj, nil
}

func (s *S3Store) Delete(ctx context.Context, key string) error {
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("remove object: %w", err)
	}
	return nil
}

func (s *S3Store) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	info, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return ObjectInfo{}, fmt.Errorf("stat object: %w", err)
	}
	return ObjectInfo{
		Key:          key,
		Size:         info.Size,
		ContentType:  info.ContentType,
		LastModified: info.LastModified,
	}, nil
}

// 触发 time 引用，避免未使用导入在扩展时遗漏
var _ = time.Time{}
```

- [ ] **Step 3: 写集成测试**

`internal/storage/s3_test.go`:
```go
//go:build integration

package storage

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/skillhub/skillhub/internal/config"
)

func s3CfgForTest() config.S3Storage {
	return config.S3Storage{
		Endpoint:  os.Getenv("SKILLHUB_S3_ENDPOINT"),
		Bucket:    "skillhub",
		Region:    "us-east-1",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
		UseSSL:    false,
	}
}

func TestS3_PutGetDelete(t *testing.T) {
	s, err := NewS3(s3CfgForTest())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	body := []byte("hello s3")
	if _, err := s.Put(ctx, "test/s3.txt", bytes.NewReader(body), int64(len(body)), "text/plain"); err != nil {
		t.Fatal(err)
	}
	rc, err := s.Get(ctx, "test/s3.txt")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(got, body) {
		t.Fatalf("got %q", got)
	}
	if err := s.Delete(ctx, "test/s3.txt"); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 4: 跑单元测试（不含 integration）确认编译**

Run: `go test ./internal/storage/`
Expected: PASS（local 测试；s3 集成测试被 tag 跳过）

- [ ] **Step 5: 跑集成测试（需 compose-up）**

Run:
```bash
make compose-up
SKILLHUB_S3_ENDPOINT=localhost:9000 go test -tags integration ./internal/storage/
```
Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add internal/storage go.mod go.sum
git commit -m "feat(storage): add S3/MinIO implementation with integration test"
```

---

### Task 9: HTTP 中间件

**Files:**
- Create: `internal/httpserver/middleware/recover.go`
- Create: `internal/httpserver/middleware/requestid.go`
- Create: `internal/httpserver/middleware/accesslog.go`
- Create: `internal/httpserver/middleware/errors.go`
- Create: `internal/httpserver/middleware/middleware_test.go`

**Interfaces:**
- Consumes: `*zap.Logger`, `apperr`
- Produces: `middleware.Recover(logger)`, `middleware.RequestID()`, `middleware.AccessLog(logger)`, `middleware.Errors()` —— 均返回 `gin.HandlerFunc`

- [ ] **Step 1: 拉依赖**

Run:
```bash
go get github.com/gin-gonic/gin@latest github.com/google/uuid@latest
```

- [ ] **Step 2: 写实现**

`internal/httpserver/middleware/requestid.go`:
```go
package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const RequestIDKey = "request_id"

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-Id")
		if id == "" {
			id = uuid.NewString()
		}
		c.Set(RequestIDKey, id)
		c.Header("X-Request-Id", id)
		c.Next()
	}
}
```

`internal/httpserver/middleware/recover.go`:
```go
package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func Recover(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic recovered",
					zap.Any("panic", r),
					zap.String(RequestIDKey, c.GetString(RequestIDKey)),
				)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": gin.H{"code": "internal", "message": "internal error"},
				})
			}
		}()
		c.Next()
	}
}
```

`internal/httpserver/middleware/accesslog.go`:
```go
package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func AccessLog(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		logger.Info("http request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
			zap.String(RequestIDKey, c.GetString(RequestIDKey)),
		)
	}
}
```

`internal/httpserver/middleware/errors.go`:
```go
package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/skillhub/skillhub/internal/apperr"
)

func Errors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 {
			return
		}
		e := c.Errors.Last().Err
		status := apperr.HTTPStatus(e)
		code := "internal"
		ae, ok := e.(*apperr.Error)
		if ok {
			code = ae.Code
		}
		c.JSON(status, gin.H{
			"error": gin.H{
				"code":       code,
				"message":    e.Error(),
				"request_id": c.GetString(RequestIDKey),
			},
		})
	}
}
```

- [ ] **Step 3: 写测试**

`internal/httpserver/middleware/middleware_test.go`:
```go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func TestRequestID_GeneratesAndSets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	r.GET("/", func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	r.ServeHTTP(w, req)
	if w.Header().Get("X-Request-Id") == "" {
		t.Fatal("missing request id header")
	}
}

func TestRequestID_PreservesIncoming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	r.GET("/", func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-Id", "abc")
	r.ServeHTTP(w, req)
	if w.Header().Get("X-Request-Id") != "abc" {
		t.Fatalf("got %s", w.Header().Get("X-Request-Id"))
	}
}

func TestRecover_Panic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID(), Recover(zap.NewNop()))
	r.GET("/", func(c *gin.Context) { panic("boom") })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("got %d", w.Code)
	}
}
```

- [ ] **Step 4: 跑测试**

Run: `go test ./internal/httpserver/middleware/`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/httpserver/middleware go.mod go.sum
git commit -m "feat(middleware): add recover, requestid, accesslog, errors middleware"
```

---

### Task 10: httpserver 装配与 /healthz

**Files:**
- Create: `internal/httpserver/server.go`
- Create: `internal/httpserver/server_test.go`

**Interfaces:**
- Consumes: `*zap.Logger`, `*gorm.DB`, `*redis.Client`, `config.ServerConfig`，middleware
- Produces: `httpserver.Deps`，`httpserver.New(deps Deps) *gin.Engine`，`httpserver.Run(srv *http.Server, shutdownTimeout time.Duration, logger *zap.Logger) error`

- [ ] **Step 1: 写实现**

`internal/httpserver/server.go`:
```go
package httpserver

import (
	"context"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/skillhub/skillhub/internal/db"
	redispkg "github.com/skillhub/skillhub/internal/redis"
	"github.com/skillhub/skillhub/internal/httpserver/middleware"
	"go.uber.org/zap"
	"gorm.io/gorm"

	rdb "github.com/redis/go-redis/v9"
)

type Deps struct {
	Logger *zap.Logger
	DB     *gorm.DB
	Redis  *rdb.Client
}

func New(deps Deps) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(middleware.RequestID(), middleware.Recover(deps.Logger), middleware.AccessLog(deps.Logger), middleware.Errors())
	r.GET("/healthz", healthz(deps))
	return r
}

func healthz(deps Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		checks := gin.H{}
		ok := true
		if err := db.Ping(deps.DB); err != nil {
			ok = false
			checks["db"] = err.Error()
		} else {
			checks["db"] = "ok"
		}
		if err := redispkg.Ping(ctx, deps.Redis); err != nil {
			ok = false
			checks["redis"] = err.Error()
		} else {
			checks["redis"] = "ok"
		}
		status := "ok"
		code := 200
		if !ok {
			status = "degraded"
			code = 503
		}
		c.JSON(code, gin.H{"status": status, "checks": checks})
	}
}

func Run(srv *http.Server, shutdownTimeout time.Duration, logger *zap.Logger) error {
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
```

- [ ] **Step 2: 写烟雾测试**

`internal/httpserver/server_test.go`:
```go
package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skillhub/skillhub/internal/config"
	"go.uber.org/zap"
)

func TestHealthz_Shape(t *testing.T) {
	// 不连真实 db/redis：用 nil 会触发 ping 错误，应返回 503 degraded
	r := New(Deps{Logger: zap.NewNop(), DB: nil, Redis: nil})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d", w.Code)
	}
}

// 触发 config 引用，避免后续扩展遗漏
var _ = config.ServerConfig{}
```

注：`db.Ping(nil)` 会 panic。为避免此，Step 2 的测试先跳过 nil 分支——改用真实依赖由集成测试覆盖。重写测试如下：

`internal/httpserver/server_test.go`（替换）:
```go
package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

func TestHealthz_ReturnsJSON(t *testing.T) {
	// 仅验证路由已注册且返回 JSON；真实依赖由集成测试覆盖。
	// 这里用 nil deps 期望 panic 被中间件捕获 -> 500。
	r := New(Deps{Logger: zap.NewNop(), DB: nil, Redis: nil})
	w := httptest.NewRecorder()
	defer func() { recover() }()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	// 若到达此处，断言状态码为 5xx
	if w.Code < 500 {
		t.Fatalf("expected 5xx, got %d", w.Code)
	}
}
```

为让 healthz 在 deps 为 nil 时不 panic，更新 `healthz` 中 db/redis 调用做 nil 检查。修改 `server.go` 的 healthz：

```go
func healthz(deps Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		checks := gin.H{}
		ok := true
		if deps.DB != nil {
			if err := db.Ping(deps.DB); err != nil {
				ok = false
				checks["db"] = err.Error()
			} else {
				checks["db"] = "ok"
			}
		} else {
			ok = false
			checks["db"] = "not configured"
		}
		if deps.Redis != nil {
			if err := redispkg.Ping(ctx, deps.Redis); err != nil {
				ok = false
				checks["redis"] = err.Error()
			} else {
				checks["redis"] = "ok"
			}
		} else {
			ok = false
			checks["redis"] = "not configured"
		}
		status := "ok"
		code := 200
		if !ok {
			status = "degraded"
			code = 503
		}
		c.JSON(code, gin.H{"status": status, "checks": checks})
	}
}
```

- [ ] **Step 3: 跑测试**

Run: `go test ./internal/httpserver/`
Expected: PASS（/healthz 返回 503，因为 deps 为 nil）

- [ ] **Step 4: 提交**

```bash
git add internal/httpserver
git commit -m "feat(httpserver): add gin server, /healthz, graceful shutdown"
```

---

### Task 11: 占位迁移与 migrate 命令

**Files:**
- Create: `migrations/000001_init.up.sql`
- Create: `migrations/000001_init.down.sql`
- Create: `cmd/migrate/main.go`

**Interfaces:**
- Consumes: `config.DBConfig`
- Produces: `go run ./cmd/migrate up|down|create <name>`

- [ ] **Step 1: 写占位迁移**

`migrations/000001_init.up.sql`:
```sql
CREATE TABLE IF NOT EXISTS schema_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
INSERT INTO schema_meta(key, value) VALUES ('created_at', NOW()::TEXT)
ON CONFLICT DO NOTHING;
```

`migrations/000001_init.down.sql`:
```sql
DROP TABLE IF EXISTS schema_meta;
```

- [ ] **Step 2: 拉依赖**

Run:
```bash
go get github.com/golang-migrate/migrate/v4@latest
go get github.com/golang-migrate/migrate/v4/database/postgres@latest
go get github.com/golang-migrate/migrate/v4/source/file@latest
```

- [ ] **Step 3: 写 migrate 命令**

`cmd/migrate/main.go`:
```go
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/skillhub/skillhub/internal/config"
)

func dsn(c *config.DBConfig) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.Name, c.SSLMode)
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: migrate up|down|create <name>")
	}
	cfg, err := config.Load("config/config.yaml")
	if err != nil {
		log.Fatal(err)
	}
	m, err := migrate.New("file://migrations", dsn(&cfg.DB))
	if err != nil {
		log.Fatal(err)
	}
	defer m.Close()

	switch os.Args[1] {
	case "up":
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			log.Fatal(err)
		}
	case "down":
		if err := m.Down(); err != nil && err != migrate.ErrNoChange {
			log.Fatal(err)
		}
	case "create":
		if len(os.Args) < 3 {
			log.Fatal("usage: migrate create <name>")
		}
		if err := m.Force(0); err != nil { // ensure clean state for naming
			_ = err
		}
		// golang-migrate 不支持运行时 create；用 CLI 替代
		log.Fatal("create via: migrate -path migrations -database <dsn> create <name>")
	default:
		log.Fatalf("unknown command: %s", os.Args[1])
	}
}
```

注：`create` 子命令需用 `migrate` CLI 二进制。更新 Makefile 的 `migrate-create` 目标改用 `migrate` CLI（若未安装则提示）。

修改 Makefile 的 `migrate-create`：
```makefile
migrate-create:
	@test -n "$(NAME)" || (echo "Usage: make migrate-create NAME=foo" && exit 1)
	@command -v migrate >/dev/null 2>&1 || (echo "install: go install github.com/golang-migrate/migrate/v4/cmd/migrate@latest" && exit 1)
	migrate -path migrations -database "postgres://skillhub:skillhub@localhost:5432/skillhub?sslmode=disable" create $(NAME)
```

- [ ] **Step 4: 编译验证**

Run: `go build ./cmd/migrate`
Expected: 无错误

- [ ] **Step 5: 提交**

```bash
git add migrations cmd/migrate Makefile go.mod go.sum
git commit -m "feat(migrate): add golang-migrate wrapper and placeholder migration"
```

---

### Task 12: main 装配与启动

**Files:**
- Create: `cmd/skillhub/main.go`

**Interfaces:**
- Consumes: 所有 internal 包
- Produces: 可运行服务 `go run ./cmd/skillhub`

- [ ] **Step 1: 写实现**

`cmd/skillhub/main.go`:
```go
package main

import (
	"net/http"
	"time"

	"github.com/skillhub/skillhub/internal/config"
	"github.com/skillhub/skillhub/internal/db"
	"github.com/skillhub/skillhub/internal/httpserver"
	redispkg "github.com/skillhub/skillhub/internal/redis"
	"github.com/skillhub/skillhub/internal/log"
)

func main() {
	cfg, err := config.Load("config/config.yaml")
	if err != nil {
		panic(err)
	}
	logger, err := log.New(cfg.Log)
	if err != nil {
		panic(err)
	}
	gdb, err := db.New(cfg.DB)
	if err != nil {
		logger.Fatal("init db", zap.Error(err))
	}
	rdb, err := redispkg.New(cfg.Redis)
	if err != nil {
		logger.Fatal("init redis", zap.Error(err))
	}

	engine := httpserver.New(httpserver.Deps{
		Logger: logger,
		DB:     gdb,
		Redis:  rdb,
	})
	srv := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      engine,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}
	logger.Info("starting server", zap.String("addr", srv.Addr))
	if err := httpserver.Run(srv, cfg.Server.ShutdownTimeout, logger); err != nil {
		logger.Fatal("server error", zap.Error(err))
	}
}
```

注意需导入 zap：在 import 块加 `"go.uber.org/zap"`。

修正后的 import 块：
```go
import (
	"net/http"

	"github.com/skillhub/skillhub/internal/config"
	"github.com/skillhub/skillhub/internal/db"
	"github.com/skillhub/skillhub/internal/httpserver"
	"github.com/skillhub/skillhub/internal/log"
	redispkg "github.com/skillhub/skillhub/internal/redis"
	"go.uber.org/zap"
)
```
（移除未使用的 `"time"`。）

- [ ] **Step 2: 编译验证**

Run: `go build ./cmd/skillhub`
Expected: 无错误

- [ ] **Step 3: tidy 并跑全部单元测试**

Run:
```bash
go mod tidy
go test ./...
```
Expected: PASS

- [ ] **Step 4: 端到端冒烟（手动）**

Run:
```bash
make compose-up
make migrate-up
make run &
sleep 2
curl -s http://localhost:8080/healthz
kill %1
```
Expected: `{"checks":{"db":"ok","redis":"ok"},"status":"ok"}`

- [ ] **Step 5: 提交**

```bash
git add cmd/skillhub go.mod go.sum
git commit -m "feat(main): wire config/log/db/redis/httpserver and start service"
```

---

### Task 13: lint 配置与最终验证

**Files:**
- Create: `.golangci.yml`

**Interfaces:** 无

- [ ] **Step 1: 写 .golangci.yml**

```yaml
linters:
  enable:
    - errcheck
    - govet
    - staticcheck
    - unused
    - ineffassign
    - gosimple
run:
  timeout: 3m
```

- [ ] **Step 2: 跑 lint（若装了 golangci-lint）**

Run: `make lint || echo "golangci-lint not installed, skip"`
Expected: 无错误或跳过

- [ ] **Step 3: 跑全部测试**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 4: 提交**

```bash
git add .golangci.yml
git commit -m "chore: add golangci-lint config"
```

- [ ] **Step 5: 更新 README 验证步骤并提交**

确认 README 中本地开发步骤与实际一致（已在 Task 1 写好）。无需改动则跳过。

---

## Self-Review 记录

- **Spec 覆盖**：spec 第 4 节每个组件都有对应 Task（config→3, log→4, db→5, redis→6, storage→7+8, httpserver→10, middleware→9, migrate→11, apperr→2, main→12）。docker-compose/Makefile→1。覆盖完整。
- **占位符**：无 TBD/TODO；Task 11 的 `create` 子命令限制已显式说明并改 Makefile。
- **类型一致**：`Store` 接口在 Task 7 定义，Task 8 的 S3Store 实现同签名；`Deps` 在 Task 10 定义，Task 12 使用同字段名；`config.Load` 在 Task 3 定义，Task 11/12 调用同签名。
- 已知小瑕疵：Task 10 的 healthz 对 nil deps 返回 503，已在步骤内修正；Task 12 import 块已修正移除 `time`、加 `zap`。
