package web

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/oyaguma3/web-gui-for-aka-only-server/internal/adminapi"
)

// fakeAdmin は管理API の代わり。
type fakeAdmin struct {
	status adminapi.Status
	err    error
}

func (f *fakeAdmin) Status(context.Context) (adminapi.Status, error) { return f.status, f.err }

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	return newTestHandlerWith(t, &fakeAdmin{status: adminapi.Status{Version: "1.0.0", SubscriberCount: 12, ClientCount: 3}})
}

func newTestHandlerWith(t *testing.T, admin AdminAPI) http.Handler {
	t.Helper()
	h, err := New(Options{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Version: "test", Admin: admin})
	if err != nil {
		t.Fatal(err)
	}
	return h.Routes()
}

func do(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestDashboard(t *testing.T) {
	h := newTestHandler(t)
	w := do(h, httptest.NewRequest("GET", "https://gui.example/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{"<title>ダッシュボード | aka-only-server 管理</title>", "/static/htmx.min.js",
		"web-gui-for-aka-only-server test", `<p class="metric">12</p>`, "<td>1.0.0</td>"} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q", want)
		}
	}
	if got := w.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q", got)
	}
	if got := w.Header().Get("Content-Security-Policy"); !strings.Contains(got, "default-src 'self'") {
		t.Errorf("Content-Security-Policy = %q", got)
	}
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q", got)
	}
}

func TestNotFound(t *testing.T) {
	h := newTestHandler(t)
	for _, path := range []string{"/nope", "/static/", "/static/nope.js"} {
		w := do(h, httptest.NewRequest("GET", "https://gui.example"+path, nil))
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d", path, w.Code)
		}
		if !strings.Contains(w.Body.String(), "ページが見つかりません。") {
			t.Errorf("%s: 404 page body: %s", path, w.Body.String())
		}
	}
}

func TestStatic(t *testing.T) {
	h := newTestHandler(t)
	for path, ctype := range map[string]string{
		"/static/htmx.min.js":  "text/javascript; charset=utf-8",
		"/static/pico.min.css": "text/css; charset=utf-8",
		"/static/app.css":      "text/css; charset=utf-8",
	} {
		w := do(h, httptest.NewRequest("GET", "https://gui.example"+path, nil))
		if w.Code != http.StatusOK {
			t.Errorf("%s: status = %d", path, w.Code)
			continue
		}
		if got := w.Header().Get("Content-Type"); got != ctype {
			t.Errorf("%s: Content-Type = %q", path, got)
		}
		etag := w.Header().Get("ETag")
		if etag == "" || w.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("%s: ETag = %q, Cache-Control = %q", path, etag, w.Header().Get("Cache-Control"))
			continue
		}

		// 同じ ETag なら 304 を返す。
		r := httptest.NewRequest("GET", "https://gui.example"+path, nil)
		r.Header.Set("If-None-Match", etag)
		if w := do(h, r); w.Code != http.StatusNotModified {
			t.Errorf("%s: conditional status = %d", path, w.Code)
		}
	}
}

func TestCrossOriginProtection(t *testing.T) {
	h := newTestHandler(t)

	r := httptest.NewRequest("POST", "https://gui.example/", nil)
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	w := do(h, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("cross-site POST: status = %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "別のサイトからの操作は受け付けません。") {
		t.Errorf("cross-site POST body: %s", w.Body.String())
	}

	// 同一オリジンの POST は通す（ここではルートがないので 404 になる）。
	r = httptest.NewRequest("POST", "https://gui.example/", nil)
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	if w := do(h, r); w.Code != http.StatusNotFound {
		t.Errorf("same-origin POST: status = %d", w.Code)
	}
}

func TestDashboardAdminError(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want string
	}{
		"unreachable": {
			err:  fmt.Errorf("wrap: %w", &net.OpError{Op: "dial", Err: fmt.Errorf("connection refused")}),
			want: "<p>管理API に接続できません。aka-only-server が起動しているか",
		},
		"unknown": {
			err:  fmt.Errorf("something odd"),
			want: "<p>aka-only-server の管理API に接続できません。</p>",
		},
		"api error": {
			err:  &adminapi.Error{Status: 500, Problem: adminapi.Problem{Cause: adminapi.CauseSystemFailure}},
			want: "管理API がエラーを返しました。",
		},
	} {
		h := newTestHandlerWith(t, &fakeAdmin{err: tc.err})
		w := do(h, httptest.NewRequest("GET", "https://gui.example/", nil))
		// 管理API に届かなくても画面自体は返す。
		if w.Code != http.StatusOK {
			t.Errorf("%s: status = %d", name, w.Code)
		}
		body := w.Body.String()
		if !strings.Contains(body, tc.want) || strings.Contains(body, `class="metric"`) {
			t.Errorf("%s: body = %s", name, body)
		}
	}
}

func TestDatetime(t *testing.T) {
	f := funcs["datetime"].(func(time.Time) string)
	if got := f(time.Time{}); got != "-" {
		t.Errorf("zero = %q", got)
	}
	if got := f(time.Date(2026, 10, 3, 15, 4, 5, 0, time.UTC)); !strings.HasPrefix(got, "2026-10-0") {
		t.Errorf("got %q", got)
	}
}
