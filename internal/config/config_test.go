package config

import (
	"log/slog"
	"slices"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	for _, name := range []string{"WEBGUI_ADDR", "WEBGUI_TLS_CERT", "WEBGUI_TLS_KEY", "WEBGUI_TLS_HOSTS", "WEBGUI_LOG_LEVEL",
		"WEBGUI_ADMIN_URL", "WEBGUI_ADMIN_CLIENT_CERT", "WEBGUI_ADMIN_CLIENT_KEY", "WEBGUI_ADMIN_SERVER_CERT"} {
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
