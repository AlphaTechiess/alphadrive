package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddress       string        `json:"listen_address"`
	DataDir             string        `json:"data_dir"`
	PublicBaseURL       string        `json:"public_base_url"`
	MaxUploadBytes      int64         `json:"max_upload_bytes"`
	MaxConcurrentUpload int           `json:"max_concurrent_uploads"`
	StorageQuotaBytes   int64         `json:"storage_quota_bytes"`
	SessionIdleTimeout  time.Duration `json:"-"`
	SessionMaxLifetime  time.Duration `json:"-"`
	SecureCookies       bool          `json:"secure_cookies"`
}

func Default() Config {
	return Config{
		ListenAddress:       "127.0.0.1:8080",
		DataDir:             "./data",
		MaxUploadBytes:      0, // 0 = unlimited (bounded only by host disk space)
		MaxConcurrentUpload: 4,
		StorageQuotaBytes:   0, // 0 = unlimited (bounded only by host disk space)
		SessionIdleTimeout:  24 * time.Hour,
		SessionMaxLifetime:  30 * 24 * time.Hour,
		SecureCookies:       true,
	}
}

func Load(path string) (Config, error) {
	c := Default()
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return c, fmt.Errorf("read config: %w", err)
		}
		if err := json.Unmarshal(data, &c); err != nil {
			return c, fmt.Errorf("parse config: %w", err)
		}
	}
	override(&c)
	if c.DataDir == "" || c.MaxConcurrentUpload <= 0 || c.MaxUploadBytes < 0 || c.StorageQuotaBytes < 0 {
		return c, fmt.Errorf("invalid configuration")
	}
	abs, err := filepath.Abs(c.DataDir)
	if err != nil {
		return c, err
	}
	c.DataDir = abs
	return c, nil
}

func override(c *Config) {
	if v := os.Getenv("ALPHADRIVE_LISTEN_ADDRESS"); v != "" {
		c.ListenAddress = v
	}
	if v := os.Getenv("ALPHADRIVE_DATA_DIR"); v != "" {
		c.DataDir = v
	}
	if v := os.Getenv("ALPHADRIVE_PUBLIC_BASE_URL"); v != "" {
		c.PublicBaseURL = strings.TrimRight(v, "/")
	}
	if v := os.Getenv("ALPHADRIVE_MAX_UPLOAD_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			c.MaxUploadBytes = n
		}
	}
	if v := os.Getenv("ALPHADRIVE_STORAGE_QUOTA_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			c.StorageQuotaBytes = n
		}
	}
	if v := os.Getenv("ALPHADRIVE_INSECURE_COOKIES"); v == "true" {
		c.SecureCookies = false
	}
}
