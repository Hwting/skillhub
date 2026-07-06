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
	Auth    AuthConfig    `mapstructure:"auth"`
}

type AuthConfig struct {
	SessionTTL     time.Duration `mapstructure:"session_ttl"`
	CookieName     string        `mapstructure:"cookie_name"`
	CookieSecure   bool          `mapstructure:"cookie_secure"`
	CookieDomain   string        `mapstructure:"cookie_domain"`
	CookieSameSite string        `mapstructure:"cookie_samesite"`
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
	if c.Auth.SessionTTL <= 0 {
		return fmt.Errorf("auth.session_ttl must be > 0")
	}
	if c.Auth.CookieName == "" {
		return fmt.Errorf("auth.cookie_name is required")
	}
	switch c.Auth.CookieSameSite {
	case "strict", "lax", "none":
	default:
		return fmt.Errorf("auth.cookie_samesite must be strict|lax|none, got %q", c.Auth.CookieSameSite)
	}
	if c.Auth.CookieSameSite == "none" && !c.Auth.CookieSecure {
		return fmt.Errorf("auth.cookie_secure must be true when cookie_samesite is none")
	}
	return nil
}
