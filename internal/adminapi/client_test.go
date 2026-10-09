package adminapi

import (
	"crypto/tls"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oyaguma3/web-gui-for-aka-only-server/internal/certs"
	"github.com/oyaguma3/web-gui-for-aka-only-server/internal/trace"
)

var discard = slog.New(slog.NewTextHandler(io.Discard, nil))

// testEnv は mTLS の管理API の代わりになるテスト用サーバーと、それにつながるクライアント。
type testEnv struct {
	client *Client
	srv    *httptest.Server
	// clientFP はクライアントが提示するはずの証明書のフィンガープリント。
	clientFP string
	dir      string
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// newTestEnv はハンドラー h を mTLS で提供するサーバーを立て、クライアントを作る。
// クライアント証明書は証明書と秘密鍵を 1 つの PEM にまとめた形（client gen-cert の出力と同じ）で渡す。
func newTestEnv(t *testing.T, h http.HandlerFunc) *testEnv {
	t.Helper()
	dir := t.TempDir()

	serverCertPEM, serverKeyPEM, err := certs.SelfSigned("aka-only-server", []string{"127.0.0.1"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	serverCert, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	clientCertPEM, clientKeyPEM, err := certs.SelfSigned("bff", nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	clientCert, err := tls.X509KeyPair(clientCertPEM, clientKeyPEM)
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewUnstartedServer(h)
	srv.Config.ErrorLog = slog.NewLogLogger(discard.Handler(), slog.LevelWarn)
	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAnyClientCert,
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	writeFile(t, filepath.Join(dir, "client.pem"), append(clientCertPEM, clientKeyPEM...))
	writeFile(t, filepath.Join(dir, "server.pem"), serverCertPEM)
	c, err := New(Options{
		BaseURL:        srv.URL + "/admin/v1",
		ClientCertFile: filepath.Join(dir, "client.pem"),
		ServerCertFile: filepath.Join(dir, "server.pem"),
		Log:            discard,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &testEnv{client: c, srv: srv, clientFP: certs.Fingerprint(clientCert.Leaf), dir: dir}
}

// closedAddr は、待ち受けていないことが確かなアドレスを返す。
func closedAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.MarshalWrite(w, v)
}

func TestStatusOverMTLS(t *testing.T) {
	var gotFP, gotPath, gotTrace string
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotTrace = r.Header.Get("X-Trace-ID")
		gotFP = certs.Fingerprint(r.TLS.PeerCertificates[0])
		writeJSON(w, 200, map[string]any{
			"version": "1.2.3", "bootId": "b", "startedAt": "2026-10-03T00:00:00Z",
			"subscriberCount": 7, "clientCount": 2, "avPlainEnabled": false,
			"avServerCertificateNotAfter": "2036-10-01T00:00:00Z",
			"futureField":                 "ignored",
		})
	})
	st, err := env.client.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/admin/v1/status" {
		t.Errorf("path = %q", gotPath)
	}
	if gotFP != env.clientFP {
		t.Error("client certificate was not presented")
	}
	// コンテキストにトレースID がなければ、呼び出しごとに採番して送る。
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(gotTrace) {
		t.Errorf("X-Trace-ID = %q", gotTrace)
	}
	if st.Version != "1.2.3" || st.SubscriberCount != 7 || st.ClientCount != 2 ||
		!st.AVServerCertificateNotAfter.Equal(time.Date(2036, 10, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("status = %+v", st)
	}
	if certs.Fingerprint(env.client.ClientCertificate()) != env.clientFP {
		t.Error("ClientCertificate mismatch")
	}
}

func TestTraceID(t *testing.T) {
	var gotTrace string
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		gotTrace = r.Header.Get("X-Trace-ID")
		w.Header().Set("X-Trace-ID", gotTrace)
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"title":"Not Found","status":404,"cause":"USER_NOT_FOUND"}`)
	})
	ctx := trace.With(t.Context(), "0123456789abcdef0123456789abcdef")
	_, err := env.client.GetSubscriber(ctx, "440100000000000")
	if gotTrace != "0123456789abcdef0123456789abcdef" {
		t.Errorf("X-Trace-ID = %q", gotTrace)
	}
	if apiErr, ok := errors.AsType[*Error](err); !ok || apiErr.TraceID != gotTrace {
		t.Errorf("err = %#v", err)
	}
}

func TestOperator(t *testing.T) {
	var calls int
	var gotOperator string
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		gotOperator = r.Header.Get("X-Operator-Id")
		if r.Method == http.MethodGet {
			writeJSON(w, 200, map[string]any{"imsi": "440100123456789"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// 操作者がなければ送らない。
	if err := env.client.DeleteSubscriber(t.Context(), "440100123456789"); !errors.Is(err, ErrNoOperator) {
		t.Errorf("without operator: err = %v", err)
	}
	// 形式に合わない操作者も送らない。
	ctx := WithOperator(t.Context(), "bad user")
	if err := env.client.DeleteSubscriber(ctx, "440100123456789"); !errors.Is(err, ErrInvalidOperator) {
		t.Errorf("invalid operator: err = %v", err)
	}
	if calls != 0 {
		t.Fatalf("requests were sent: %d", calls)
	}

	ctx = WithOperator(t.Context(), "alice@example")
	if err := env.client.DeleteSubscriber(ctx, "440100123456789"); err != nil {
		t.Fatal(err)
	}
	if gotOperator != "alice@example" {
		t.Errorf("X-Operator-Id = %q", gotOperator)
	}

	// 参照系は操作者がなくても送れる。
	if _, err := env.client.GetSubscriber(t.Context(), "440100123456789"); err != nil {
		t.Fatal(err)
	}
	if gotOperator != "" {
		t.Errorf("read without operator: X-Operator-Id = %q", gotOperator)
	}
}

func TestRequestBodies(t *testing.T) {
	type captured struct {
		method, path, ctype, query string
		body                       map[string]any
	}
	var got captured
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		got = captured{method: r.Method, path: r.URL.Path, ctype: r.Header.Get("Content-Type"), query: r.URL.RawQuery}
		got.body = nil
		if r.Body != nil {
			json.UnmarshalRead(r.Body, &got.body)
		}
		writeJSON(w, 200, map[string]any{"imsi": "440100123456789"})
	})
	ctx := WithOperator(t.Context(), "alice")

	// 登録: 空の SQN などは送らず管理API の既定値に任せる。許可クライアントは空でも [] を送る。
	if _, err := env.client.CreateSubscriber(ctx, SubscriberCreate{
		IMSI: "440100123456789", Ki: "465b5ce8b199b49faa5f0a2ee238a6bc", OPc: "cd63cb71954a9f4e48a5994e37a02baf",
	}); err != nil {
		t.Fatal(err)
	}
	if got.method != "POST" || got.path != "/admin/v1/subscribers" || got.ctype != "application/json" {
		t.Errorf("create: %+v", got)
	}
	if _, ok := got.body["sqn"]; ok {
		t.Errorf("create: sqn must be omitted: %v", got.body)
	}
	if ids, ok := got.body["allowedClientIds"].([]any); !ok || len(ids) != 0 {
		t.Errorf("create: allowedClientIds = %#v", got.body["allowedClientIds"])
	}

	// 変更: 指定した項目だけを送る。空のスライスは「全て外す」として送る。
	if _, err := env.client.UpdateSubscriber(ctx, "440100123456789", SubscriberUpdate{
		AllowPlain: new(false), AllowedClientIDs: new([]int64{}),
	}); err != nil {
		t.Fatal(err)
	}
	if got.method != "PATCH" || got.ctype != "application/merge-patch+json" || len(got.body) != 2 {
		t.Errorf("update: %+v", got)
	}
	if v, ok := got.body["allowPlain"].(bool); !ok || v {
		t.Errorf("update: allowPlain = %#v", got.body["allowPlain"])
	}

	// クエリ: ゼロ値は送らない。after=0 は送る。
	env.client.ListSubscribers(t.Context(), ListSubscribersParams{Prefix: "44010", Limit: 20})
	if got.query != "limit=20&prefix=44010" {
		t.Errorf("list subscribers query = %q", got.query)
	}
	env.client.ListLogs(t.Context(), ListLogsParams{After: new(int64(0))})
	if got.query != "after=0" {
		t.Errorf("list logs query = %q", got.query)
	}
	env.client.ListAuditLogs(t.Context(), ListAuditLogsParams{})
	if got.path != "/admin/v1/audit-logs" || got.query != "" {
		t.Errorf("list audit logs: %+v", got)
	}
	env.client.ReplaceAVClientCertificate(ctx, 3, "PEM")
	if got.method != "PUT" || got.path != "/admin/v1/clients/3/certificate" || got.body["certPem"] != "PEM" {
		t.Errorf("replace client certificate: %+v", got)
	}
}

func TestErrors(t *testing.T) {
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/admin/v1/subscribers/440100000000000":
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"title":"Not Found","status":404,"cause":"USER_NOT_FOUND"}`)
		case "/admin/v1/subscribers":
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"title":"Bad Request","status":400,"cause":"MANDATORY_IE_INCORRECT",`+
				`"invalidParams":[{"param":"ki","reason":"must be 32 hex digits"}]}`)
		default:
			http.NotFound(w, r)
		}
	})

	_, err := env.client.GetSubscriber(t.Context(), "440100000000000")
	if CauseOf(err) != CauseUserNotFound || IsUnavailable(err) {
		t.Errorf("not found: err = %v", err)
	}

	_, err = env.client.CreateSubscriber(WithOperator(t.Context(), "alice"), SubscriberCreate{IMSI: "440100000000000"})
	apiErr, ok := errors.AsType[*Error](err)
	if !ok || apiErr.Status != 400 || apiErr.Problem.Cause != CauseMandatoryIEIncorrect ||
		!slices.Equal(apiErr.Problem.InvalidParams, []InvalidParam{{"ki", "must be 32 hex digits"}}) {
		t.Errorf("bad request: err = %#v", err)
	}

	// ProblemDetails でない応答もステータスで扱える。
	_, err = env.client.GetAVClient(t.Context(), 9)
	if apiErr, ok := errors.AsType[*Error](err); !ok || apiErr.Status != 404 || apiErr.Problem.Cause != "" {
		t.Errorf("plain 404: err = %v", err)
	}
}

func TestServerCertificateMismatch(t *testing.T) {
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{})
	})
	// 別の自己署名証明書を信頼させると、接続できない。
	otherPEM, _, err := certs.SelfSigned("other", []string{"127.0.0.1"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(env.dir, "other.pem"), otherPEM)
	c, err := New(Options{
		BaseURL:        env.srv.URL + "/admin/v1",
		ClientCertFile: filepath.Join(env.dir, "client.pem"),
		ServerCertFile: filepath.Join(env.dir, "other.pem"),
		Log:            discard,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Status(t.Context())
	if _, ok := errors.AsType[*tls.CertificateVerificationError](err); !ok || !IsUnavailable(err) {
		t.Errorf("err = %v", err)
	}

	// 届かない宛先も IsUnavailable。
	c.base.Host = closedAddr(t)
	if _, err := c.Status(t.Context()); !IsUnavailable(err) {
		t.Errorf("unreachable: err = %v", err)
	}
}

func TestNewErrors(t *testing.T) {
	env := newTestEnv(t, func(http.ResponseWriter, *http.Request) {})
	client := filepath.Join(env.dir, "client.pem")
	server := filepath.Join(env.dir, "server.pem")
	for name, opts := range map[string]Options{
		"http url":       {BaseURL: "http://aka-only-server:9443/admin/v1", ClientCertFile: client, ServerCertFile: server},
		"no client cert": {BaseURL: env.srv.URL, ClientCertFile: filepath.Join(env.dir, "none.pem"), ServerCertFile: server},
		"no server cert": {BaseURL: env.srv.URL, ClientCertFile: client, ServerCertFile: filepath.Join(env.dir, "none.pem")},
		"server not pem": {BaseURL: env.srv.URL, ClientCertFile: client, ServerCertFile: client + "x"},
	} {
		if name == "server not pem" {
			writeFile(t, opts.ServerCertFile, []byte("not a pem"))
		}
		if _, err := New(opts); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	// 秘密鍵を別ファイルで渡す形も使える。
	certPEM, keyPEM, err := certs.SelfSigned("bff", nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(env.dir, "c.pem"), certPEM)
	writeFile(t, filepath.Join(env.dir, "k.pem"), keyPEM)
	if _, err := New(Options{
		BaseURL: env.srv.URL, ClientCertFile: filepath.Join(env.dir, "c.pem"),
		ClientKeyFile: filepath.Join(env.dir, "k.pem"), ServerCertFile: server, Log: discard,
	}); err != nil {
		t.Errorf("separate key file: %v", err)
	}
}

func TestDiagnose(t *testing.T) {
	ok := func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]any{}) }

	// 別の証明書を信頼している。
	env := newTestEnv(t, ok)
	otherPEM, _, err := certs.SelfSigned("other", []string{"127.0.0.1"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(env.dir, "server.pem"), otherPEM)
	c, err := New(Options{BaseURL: env.srv.URL, ClientCertFile: filepath.Join(env.dir, "client.pem"),
		ServerCertFile: filepath.Join(env.dir, "server.pem"), Log: discard})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Status(t.Context())
	if got := Diagnose(err); !strings.Contains(got, "WEBGUI_ADMIN_SERVER_CERT") {
		t.Errorf("unknown authority: %q (err %v)", got, err)
	}

	// サーバー証明書の SAN に接続先がない。
	env = newTestEnv(t, ok)
	env.client.base.Host = strings.Replace(env.client.base.Host, "127.0.0.1", "localhost", 1)
	_, err = env.client.Status(t.Context())
	if got := Diagnose(err); !strings.Contains(got, "AKA_ADMIN_TLS_HOSTS") {
		t.Errorf("hostname: %q (err %v)", got, err)
	}

	// クライアント証明書を拒否された（aka-only-server と同じく VerifyConnection で拒否する）。
	env = newTestEnv(t, ok)
	env.srv.TLS.VerifyConnection = func(tls.ConnectionState) error { return errors.New("not a configured admin client") }
	_, err = env.client.Status(t.Context())
	if got := Diagnose(err); !strings.Contains(got, "AKA_ADMIN_CLIENTS") {
		t.Errorf("rejected client: %q (err %v)", got, err)
	}

	// 接続できない。
	env.client.base.Host = closedAddr(t)
	_, err = env.client.Status(t.Context())
	if got := Diagnose(err); !strings.Contains(got, "起動しているか") {
		t.Errorf("dial: %q (err %v)", got, err)
	}

	// 名前を解決できない。
	dnsErr := fmt.Errorf("wrap: %w", &net.DNSError{Name: "aka-only-server", IsNotFound: true})
	if got := Diagnose(dnsErr); !strings.Contains(got, "（aka-only-server）") || !strings.Contains(got, "起動しているか") {
		t.Errorf("dns: %q", got)
	}

	if got := Diagnose(&Error{Status: 404}); got != "" {
		t.Errorf("api error: %q", got)
	}
}
