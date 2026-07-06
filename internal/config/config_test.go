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
auth:
  session_ttl: 24h
  cookie_name: sid
  cookie_secure: false
  cookie_domain: ""
  cookie_samesite: lax
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

func TestValidate_InvalidSameSite(t *testing.T) {
	c, _ := Load(writeTemp(t, validYAML))
	c.Auth.CookieSameSite = "bogus"
	if err := c.Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidate_MissingCookieName(t *testing.T) {
	c, _ := Load(writeTemp(t, validYAML))
	c.Auth.CookieName = ""
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
