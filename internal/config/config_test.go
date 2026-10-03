package config

import (
	"log/slog"
	"slices"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	for _, name := range []string{"WEBGUI_ADDR", "WEBGUI_TLS_CERT", "WEBGUI_TLS_KEY", "WEBGUI_TLS_HOSTS", "WEBGUI_LOG_LEVEL",
		"WEBGUI_ADMIN_URL", "WEBGUI_ADMIN_CLIENT_CERT", "WEBGUI_ADMIN_CLIENT_KEY", "WEBGUI_ADMIN_SERVER_CERT",
		"WEBGUI_VALKEY_ADDR", "WEBGUI_VALKEY_PASSWORD", "WEBGUI_INITIAL_ADMIN_ID", "WEBGUI_INITIAL_ADMIN_PASSWORD",
		"WEBGUI_SESSION_IDLE_TIMEOUT", "WEBGUI_SESSION_MAX_AGE", "WEBGUI_LOGIN_MAX_FAILURES", "WEBGUI_LOGIN_LOCK_DURATION",
		"WEBGUI_AUDIT_MAX"} {
		t.Setenv(name, "")
	}
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != ":8443" || c.TLSCertFile != "/data/tls/cert.pem" || c.TLSKeyFile != "/data/tls/key.pem" {
		t.Errorf("got %+v", c)
	}
	if !slices.Equal(c.TLSHosts, []string{"localhost", "127.0.0.1"}) {
		t.Errorf("TLSHosts = %v", c.TLSHosts)
	}
	if c.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel = %v", c.LogLevel)
	}
	if c.AdminURL != "https://aka-only-server:9443/admin/v1" || c.AdminClientCertFile != "/certs/admin-client.pem" ||
		c.AdminClientKeyFile != "" || c.AdminServerCertFile != "/certs/admin-server.pem" {
		t.Errorf("admin: got %+v", c)
	}
	if c.ValkeyAddr != "valkey:6379" || c.SessionIdleTimeout != 30*time.Minute || c.SessionMaxAge != 12*time.Hour ||
		c.LoginMaxFailures != 5 || c.LoginLockDuration != 15*time.Minute || c.AuditMaxLen != 10000 {
		t.Errorf("session: got %+v", c)
	}
	// 最初の管理者がなければ起動しない。
	if err := c.CheckServe(); err == nil {
		t.Error("CheckServe without initial admin: want error")
	}
}

func TestLoadSessionSettings(t *testing.T) {
	t.Setenv("WEBGUI_INITIAL_ADMIN_ID", "root")
	t.Setenv("WEBGUI_INITIAL_ADMIN_PASSWORD", "owner-password-123")
	t.Setenv("WEBGUI_SESSION_IDLE_TIMEOUT", "10m")
	t.Setenv("WEBGUI_SESSION_MAX_AGE", "8h")
	t.Setenv("WEBGUI_LOGIN_MAX_FAILURES", "3")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.SessionIdleTimeout != 10*time.Minute || c.SessionMaxAge != 8*time.Hour || c.LoginMaxFailures != 3 {
		t.Errorf("got %+v", c)
	}
	if err := c.CheckServe(); err != nil {
		t.Errorf("CheckServe: %v", err)
	}

	t.Setenv("WEBGUI_SESSION_IDLE_TIMEOUT", "9h")
	if c, _ := Load(); c.CheckServe() == nil {
		t.Error("idle > max age: want error")
	}
	for name, v := range map[string]string{
		"WEBGUI_SESSION_MAX_AGE":    "forever",
		"WEBGUI_LOGIN_MAX_FAILURES": "0",
		"WEBGUI_AUDIT_MAX":          "-1",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, v)
			if _, err := Load(); err == nil {
				t.Errorf("%s=%s: want error", name, v)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	t.Setenv("WEBGUI_ADDR", "100.64.0.1:443")
	t.Setenv("WEBGUI_TLS_HOSTS", " gui.example.ts.net , 100.64.0.1,, ")
	t.Setenv("WEBGUI_LOG_LEVEL", "debug")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != "100.64.0.1:443" {
		t.Errorf("Addr = %q", c.Addr)
	}
	if !slices.Equal(c.TLSHosts, []string{"gui.example.ts.net", "100.64.0.1"}) {
		t.Errorf("TLSHosts = %v", c.TLSHosts)
	}
	if c.LogLevel != slog.LevelDebug {
		t.Errorf("LogLevel = %v", c.LogLevel)
	}

	t.Setenv("WEBGUI_LOG_LEVEL", "verbose")
	if _, err := Load(); err == nil {
		t.Error("bad log level: want error")
	}
}
