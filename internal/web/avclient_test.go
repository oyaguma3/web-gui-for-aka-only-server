package web

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

const testCertPEM = "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"

// multipartRequest はファイルつきのフォームを送るリクエストを作る。
func multipartRequest(t *testing.T, path string, cookie *http.Cookie, fields map[string]string, files map[string]string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	for k, v := range files {
		fw, err := mw.CreateFormFile(k, k+".pem")
		if err != nil {
			t.Fatal(err)
		}
		fw.Write([]byte(v))
	}
	mw.Close()
	r := httptest.NewRequest("POST", "https://gui.example"+path, &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	if cookie != nil {
		r.AddCookie(cookie)
	}
	return hx(r)
}

func TestAVClients(t *testing.T) {
	e := newSubscriberEnv(t)

	// 一般ユーザーは一覧と詳細を見られるが、登録や変更の操作は出ない。
	id := e.seedClient(t, "radius")
	body := do(e.h, request("GET", "/av-clients", e.user, nil)).Body.String()
	if !strings.Contains(body, `href="/av-clients/1"`) || strings.Contains(body, `hx-post="/av-clients"`) {
		t.Errorf("user list: %s", body)
	}
	body = do(e.h, request("GET", "/av-clients/1", e.user, nil)).Body.String()
	if !strings.Contains(body, "radius") || strings.Contains(body, "/update") || strings.Contains(body, "/delete") {
		t.Errorf("user detail: %s", body)
	}
	for _, path := range []string{"/av-clients", "/av-clients/1/update", "/av-clients/1/certificate", "/av-clients/1/delete"} {
		if w := do(e.h, hx(request("POST", path, e.user, url.Values{"name": {"x"}}))); w.Code != http.StatusForbidden {
			t.Errorf("user POST %s: %d", path, w.Code)
		}
	}

	// 登録: ファイルで渡す。
	w := do(e.h, multipartRequest(t, "/av-clients", e.owner,
		map[string]string{"name": "radius-2", "enabled": "on", "network_name": "WLAN"},
		map[string]string{"cert_file": "-----BEGIN CERTIFICATE-----\nZZZZ\n-----END CERTIFICATE-----\n"}))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "radius-2（#2）を登録しました。") {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	if c := e.admin.clients[2]; !strings.Contains(c.CertPEM, "ZZZZ") || !c.Enabled || c.NetworkName != "WLAN" {
		t.Errorf("created = %+v", c)
	}
	if got := e.admin.lastCallOf("create-client"); got != "create-client root" {
		t.Errorf("operator: %q", got)
	}
	// 同じ証明書、証明書でないもの、名前なし。
	w = do(e.h, multipartRequest(t, "/av-clients", e.owner, map[string]string{"name": "dup", "cert_pem": e.admin.clients[id].CertPEM}, nil))
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "既に別の AVクライアントに登録されています") {
		t.Errorf("duplicate: %d", w.Code)
	}
	w = do(e.h, multipartRequest(t, "/av-clients", e.owner, map[string]string{"name": "", "cert_pem": "hello"}, nil))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "名前は 1〜64 バイト") ||
		!strings.Contains(w.Body.String(), "PEM 形式の証明書") || !strings.Contains(w.Body.String(), ">hello</textarea>") {
		t.Errorf("invalid: %d %s", w.Code, w.Body.String())
	}

	// 変更: 無効にする。
	w = do(e.h, hx(request("POST", "/av-clients/1/update", e.owner, url.Values{"name": {"radius-a"}, "network_name": {""}})))
	if w.Code != http.StatusOK || e.admin.clients[1].Enabled || e.admin.clients[1].Name != "radius-a" {
		t.Errorf("update: %d %+v", w.Code, e.admin.clients[1])
	}
	// 証明書の差し替え（貼り付け）。
	w = do(e.h, multipartRequest(t, "/av-clients/1/certificate", e.owner, map[string]string{"cert_pem": testCertPEM}, nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "証明書を差し替えました") {
		t.Errorf("replace: %d %s", w.Code, w.Body.String())
	}
	// 削除すると一覧へ移動する。
	w = do(e.h, hx(request("POST", "/av-clients/1/delete", e.owner, url.Values{})))
	if w.Header().Get("HX-Redirect") != "/av-clients?deleted=1" {
		t.Errorf("delete: %d %v", w.Code, w.Header())
	}
	if w := do(e.h, request("GET", "/av-clients/1", e.owner, nil)); w.Code != http.StatusNotFound {
		t.Errorf("after delete: %d", w.Code)
	}
}

func TestServerCertificate(t *testing.T) {
	e := newSubscriberEnv(t)

	body := do(e.h, request("GET", "/av-server-certificate", e.user, nil)).Body.String()
	if !strings.Contains(body, "自己署名") || !strings.Contains(body, "CN=aka-only-server") || strings.Contains(body, `hx-post="/av-server-certificate"`) {
		t.Errorf("user view: %s", body)
	}
	w := do(e.h, request("GET", "/av-server-certificate.pem", e.user, nil))
	if w.Code != http.StatusOK || !strings.HasPrefix(w.Body.String(), "-----BEGIN CERTIFICATE-----") ||
		!strings.Contains(w.Header().Get("Content-Disposition"), "aka-av-server.pem") {
		t.Errorf("download: %d %v", w.Code, w.Header())
	}
	if w := do(e.h, hx(request("POST", "/av-server-certificate/reset", e.user, url.Values{}))); w.Code != http.StatusForbidden {
		t.Errorf("user reset: %d", w.Code)
	}

	// 秘密鍵がない: 証明書の入力は戻し、秘密鍵は戻さない。
	secretKey := "-----BEGIN PRIVATE KEY-----\nSECRET\n-----END PRIVATE KEY-----\n"
	w = do(e.h, multipartRequest(t, "/av-server-certificate", e.owner, map[string]string{"cert_pem": testCertPEM, "key_pem": "oops"}, nil))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "PEM 形式の秘密鍵") || !strings.Contains(w.Body.String(), "MIIB") {
		t.Errorf("missing key: %d %s", w.Code, w.Body.String())
	}
	w = do(e.h, multipartRequest(t, "/av-server-certificate", e.owner, map[string]string{"cert_pem": testCertPEM}, map[string]string{"key_file": secretKey}))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "持ち込み") || strings.Contains(w.Body.String(), "SECRET") {
		t.Errorf("replace: %d %s", w.Code, w.Body.String())
	}
	if got := e.admin.lastCallOf("replace-server-cert"); got != "replace-server-cert root" {
		t.Errorf("operator: %q", got)
	}
	w = do(e.h, hx(request("POST", "/av-server-certificate/reset", e.owner, url.Values{})))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "作り直しました") || !strings.Contains(w.Body.String(), "自己署名") {
		t.Errorf("reset: %d", w.Code)
	}
}

func TestExpiry(t *testing.T) {
	f := funcs["expiry"].(func(time.Time) *expiryState)
	for name, tc := range map[string]struct {
		in   time.Time
		want string
	}{
		"zero":    {time.Time{}, ""},
		"expired": {time.Now().Add(-time.Hour), "expired"},
		"soon":    {time.Now().Add(10 * 24 * time.Hour), "expiring"},
		"later":   {time.Now().Add(90 * 24 * time.Hour), ""},
	} {
		got := f(tc.in)
		if (tc.want == "" && got != nil) || (tc.want != "" && (got == nil || got.Class != tc.want)) {
			t.Errorf("%s: %+v", name, got)
		}
	}
	if got := f(time.Now().Add(10*24*time.Hour + time.Hour)); got.Label != fmt.Sprintf("あと %d 日", 10) {
		t.Errorf("label = %q", got.Label)
	}
}
