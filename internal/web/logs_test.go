package web

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oyaguma3/web-gui-for-aka-only-server/internal/adminapi"
	"github.com/oyaguma3/web-gui-for-aka-only-server/internal/auth"
)

func (e *subscriberEnv) addLogs(n int, level string) {
	for range n {
		seq := int64(len(e.admin.logs) + 1)
		e.admin.logs = append(e.admin.logs, adminapi.LogEntry{
			Seq: seq, Time: time.Now().UTC(), Level: level, Msg: fmt.Sprintf("message %d", seq),
			Attrs: map[string]any{"client_id": float64(seq), "listener": "av-mtls"},
		})
	}
}

func TestLogs(t *testing.T) {
	e := newSubscriberEnv(t)
	e.addLogs(3, "INFO")
	e.addLogs(1, "WARN")

	body := do(e.h, request("GET", "/logs", e.user, nil)).Body.String()
	// 新しい順に並べる。属性は key=value で出す。
	i4, i1 := strings.Index(body, "message 4"), strings.Index(body, "message 1")
	if i4 < 0 || i1 < 0 || i4 > i1 || !strings.Contains(body, "client_id=1 listener=av-mtls") ||
		!strings.Contains(body, `/logs/tail?after=4&amp;boot=boot-1`) {
		t.Errorf("logs page: %s", body)
	}
	body = do(e.h, request("GET", "/logs?level=WARN", e.user, nil)).Body.String()
	if !strings.Contains(body, "message 4") || strings.Contains(body, "message 3") {
		t.Errorf("filtered: %s", body)
	}

	// 続き: 新しいログを先頭に差し込み、取得位置を進める。
	e.addLogs(2, "ERROR")
	w := do(e.h, hx(request("GET", "/logs/tail?after=4&boot=boot-1", e.user, nil)))
	body = w.Body.String()
	if w.Code != http.StatusOK || !strings.HasPrefix(body, `<div id="log-poller" hx-get="/logs/tail?after=6&amp;boot=boot-1`) ||
		!strings.Contains(body, `hx-swap-oob="afterbegin:#log-rows"`) || !strings.Contains(body, "message 6") || strings.Contains(body, "message 4") {
		t.Errorf("tail: %d %s", w.Code, body)
	}
	// 新しいログがなければ、行は返さない。
	if body := do(e.h, hx(request("GET", "/logs/tail?after=6&boot=boot-1", e.user, nil))).Body.String(); strings.Contains(body, "<tbody") {
		t.Errorf("tail without new logs: %s", body)
	}
	// aka-only-server が再起動していたら読み込み直させる。
	if w := do(e.h, hx(request("GET", "/logs/tail?after=6&boot=old-boot", e.user, nil))); w.Header().Get("HX-Refresh") != "true" {
		t.Errorf("boot changed: %v", w.Header())
	}
}

// countingAuth は Authenticate と AuthenticatePassive の呼び出しを数える。
type countingAuth struct {
	*auth.Service
	active, passive atomic.Int32
}

func (c *countingAuth) Authenticate(ctx context.Context, token string) (auth.Account, error) {
	c.active.Add(1)
	return c.Service.Authenticate(ctx, token)
}

func (c *countingAuth) AuthenticatePassive(ctx context.Context, token string) (auth.Account, error) {
	c.passive.Add(1)
	return c.Service.AuthenticatePassive(ctx, token)
}

func TestLogsTailDoesNotExtendSession(t *testing.T) {
	e := newSubscriberEnv(t)
	ca := &countingAuth{Service: e.auth}
	h, err := New(Options{Log: discard, Version: "test", Admin: e.admin, Auth: ca})
	if err != nil {
		t.Fatal(err)
	}
	routes := h.Routes()
	do(routes, hx(request("GET", "/logs/tail?after=0&boot=boot-1", e.user, nil)))
	if ca.passive.Load() != 1 || ca.active.Load() != 0 {
		t.Errorf("tail: passive %d, active %d", ca.passive.Load(), ca.active.Load())
	}
	do(routes, request("GET", "/logs", e.user, nil))
	if ca.active.Load() != 1 {
		t.Errorf("logs page: active %d", ca.active.Load())
	}
}

func TestAudit(t *testing.T) {
	e := newSubscriberEnv(t)
	for i := range 60 {
		e.admin.audit = append(e.admin.audit, adminapi.AuditLogEntry{
			ID: fmt.Sprintf("%010d-0", i+1), Time: time.Now().UTC(), Operator: "bob", MgmtClient: "bff",
			Action: adminapi.AuditSubscriberCreate, Target: fmt.Sprintf("44010000000%04d", i),
			Detail: map[string]any{"sqn": "000000000000", "amf": "8000"},
		})
	}

	if w := do(e.h, request("GET", "/audit", e.user, nil)); w.Code != http.StatusForbidden {
		t.Errorf("user: %d", w.Code)
	}
	body := html.UnescapeString(do(e.h, request("GET", "/audit", e.owner, nil)).Body.String())
	if !strings.Contains(body, "加入者の登録") || !strings.Contains(body, "440100000000059") || strings.Contains(body, "440100000000009") ||
		!strings.Contains(body, `{"amf":"8000","sqn":"000000000000"}`) || !strings.Contains(body, "before=0000000011-0") {
		t.Errorf("server audit: %s", body)
	}
	// さらに古いもの: 行を末尾に足し、続きがなければボタンを消す。
	w := do(e.h, hx(request("GET", "/audit?source=server&before=0000000011-0", e.owner, nil)))
	body = w.Body.String()
	if !strings.HasPrefix(body, `<div id="audit-pager">`) || strings.Contains(body, "さらに古いものを表示") ||
		!strings.Contains(body, `hx-swap-oob="beforeend:#audit-rows"`) ||
		!strings.Contains(body, "440100000000000") || strings.Contains(body, "<html") {
		t.Errorf("more: %s", body)
	}

	// BFF の監査ログ（ログインやアカウントの操作）。
	body = do(e.h, request("GET", "/audit?source=bff", e.owner, nil)).Body.String()
	if !strings.Contains(body, "アカウントの作成") || !strings.Contains(body, "パスワードの変更") || !strings.Contains(body, "ログイン") {
		t.Errorf("bff audit: %s", body)
	}
}

func TestDashboardDetails(t *testing.T) {
	e := newSubscriberEnv(t)
	e.addLogs(12, "INFO")
	id := e.seedClient(t, "radius-old")
	c := e.admin.clients[id]
	c.NotAfter = time.Now().Add(5 * 24 * time.Hour)
	e.admin.clients[id] = c
	e.seedClient(t, "radius-new")

	body := do(e.h, request("GET", "/", e.user, nil)).Body.String()
	if !strings.Contains(body, "直近のログ") || !strings.Contains(body, "message 12") || strings.Contains(body, "message 2<") {
		t.Errorf("logs: %s", body)
	}
	if !strings.Contains(body, "radius-old</a> の証明書の有効期限") || strings.Contains(body, "radius-new</a> の証明書") ||
		!strings.Contains(body, `class="expiring"`) || strings.Contains(body, `<mark class="">`) {
		t.Errorf("expiring clients: %s", body)
	}
}
