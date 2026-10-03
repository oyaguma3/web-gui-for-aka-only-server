package web

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/oyaguma3/web-gui-for-aka-only-server/internal/adminapi"
	"github.com/oyaguma3/web-gui-for-aka-only-server/internal/auth"
)

const (
	testKi  = "465b5ce8b199b49faa5f0a2ee238a6bc"
	testOPc = "cd63cb71954a9f4e48a5994e37a02baf"
)

// subscriberEnv は加入者の画面のテスト環境。最初の管理者（root）と一般ユーザー（bob）がログインしている。
type subscriberEnv struct {
	*testEnv
	admin       *fakeAdmin
	owner, user *http.Cookie
}

func newSubscriberEnv(t *testing.T) *subscriberEnv {
	t.Helper()
	fa := newFakeAdmin()
	env := newTestEnv(t, fa)
	if err := env.auth.CreateAccount(t.Context(), auth.Account{ID: ownerID, Role: auth.RoleOwner}, "bob", auth.RoleUser, "bob-initial-pw"); err != nil {
		t.Fatal(err)
	}
	user := env.loginAs(t, "bob", "bob-initial-pw")
	w := do(env.h, request("POST", "/password", user, url.Values{"current": {"bob-initial-pw"}, "password": {"bob-own-pass"}, "confirm": {"bob-own-pass"}}))
	return &subscriberEnv{testEnv: env, admin: fa, owner: env.loginAs(t, ownerID, ownerPW), user: sessionCookieOf(w)}
}

func hx(r *http.Request) *http.Request {
	r.Header.Set("HX-Request", "true")
	return r
}

func (e *subscriberEnv) seed(t *testing.T, imsi string, clients ...int64) {
	t.Helper()
	if _, err := e.admin.CreateSubscriber(t.Context(), adminapi.SubscriberCreate{
		IMSI: imsi, Ki: testKi, OPc: testOPc, AllowedClientIDs: clients,
	}); err != nil {
		t.Fatal(err)
	}
}

func (e *subscriberEnv) seedClient(t *testing.T, name string) int64 {
	t.Helper()
	c, err := e.admin.CreateAVClient(t.Context(), adminapi.AVClientCreate{
		Name: name, CertPEM: "-----BEGIN CERTIFICATE-----\n" + name + "\n-----END CERTIFICATE-----\n", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return c.ID
}

func TestSubscriberList(t *testing.T) {
	e := newSubscriberEnv(t)
	radius := e.seedClient(t, "radius")
	for i := range 55 {
		e.seed(t, fmt.Sprintf("0010100000%05d", i), radius)
	}
	e.seed(t, "440100123456789")

	body := do(e.h, request("GET", "/subscribers", e.user, nil)).Body.String()
	if !strings.Contains(body, "全 56 件") || !strings.Contains(body, `href="/subscribers/001010000000000"`) ||
		!strings.Contains(body, "radius") || !strings.Contains(body, "cursor=001010000000049") {
		t.Errorf("list page: %s", body)
	}
	body = do(e.h, request("GET", "/subscribers?cursor=001010000000049", e.user, nil)).Body.String()
	if !strings.Contains(body, "440100123456789") || strings.Contains(body, "cursor=") || !strings.Contains(body, "先頭へ") {
		t.Errorf("page 2: %s", body)
	}
	body = do(e.h, request("GET", "/subscribers?prefix=4401", e.user, nil)).Body.String()
	if !strings.Contains(body, "4401 で始まる加入者 1 件") {
		t.Errorf("prefix: %s", body)
	}
	if w := do(e.h, request("GET", "/subscribers?prefix=44a", e.user, nil)); w.Code != http.StatusBadRequest ||
		!strings.Contains(w.Body.String(), "15 桁までの数字") {
		t.Errorf("bad prefix: %d", w.Code)
	}
}

func TestSubscriberCreate(t *testing.T) {
	e := newSubscriberEnv(t)
	radius := e.seedClient(t, "radius")

	body := do(e.h, request("GET", "/subscribers/new", e.user, nil)).Body.String()
	if !strings.Contains(body, fmt.Sprintf(`name="clients" value="%d"`, radius)) || !strings.Contains(body, `value="000000000000"`) {
		t.Errorf("new form: %s", body)
	}

	// 入力の誤り: 項目ごとに示し、入力（Ki / OPc を含む）を残す。
	form := url.Values{"imsi": {"12"}, "ki": {testKi}, "opc": {"xyz"}, "sqn": {"000000000000"}, "amf": {"8000"}, "sqn_type": {"inc32"}}
	w := do(e.h, request("POST", "/subscribers", e.user, form))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "IMSI は 5〜15 桁の数字") ||
		!strings.Contains(w.Body.String(), "OPc は 16 進 32 桁") || !strings.Contains(w.Body.String(), testKi) {
		t.Errorf("invalid: %d %s", w.Code, w.Body.String())
	}
	if len(e.admin.subs) != 0 {
		t.Error("invalid input reached the admin api")
	}

	form = url.Values{"imsi": {"440100123456789"}, "ki": {strings.ToUpper(testKi)}, "opc": {testOPc}, "sqn": {"00000000001F"},
		"amf": {"8000"}, "sqn_type": {"inc33"}, "allow_plain": {"on"}, "clients": {fmt.Sprint(radius)}}
	w = do(e.h, request("POST", "/subscribers", e.user, form))
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/subscribers/440100123456789?created=1" {
		t.Fatalf("create: %d %q", w.Code, w.Header().Get("Location"))
	}
	if got := e.admin.lastCall(); got != "create-subscriber bob" {
		t.Errorf("operator: %q", got)
	}
	sub := e.admin.subs["440100123456789"]
	if sub.SQN != "00000000001f" || sub.SQNType != "inc33" || !sub.AllowPlain || !slices.Equal(sub.AllowedClientIDs, []int64{radius}) ||
		e.admin.keys["440100123456789"].Ki != testKi {
		t.Errorf("created = %+v", sub)
	}
	if body := do(e.h, request("GET", w.Header().Get("Location"), e.user, nil)).Body.String(); !strings.Contains(body, "加入者を登録しました。") {
		t.Error("created message")
	}

	// 同じ IMSI は管理API のエラーを日本語で示す。
	w = do(e.h, request("POST", "/subscribers", e.user, form))
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "既に登録されています") {
		t.Errorf("duplicate: %d", w.Code)
	}
}

func TestSubscriberDetailPermissions(t *testing.T) {
	e := newSubscriberEnv(t)
	e.seed(t, "440100123456789")

	// 一般ユーザーには Ki / OPc と認証パラメーターの操作を出さない。
	body := do(e.h, request("GET", "/subscribers/440100123456789", e.user, nil)).Body.String()
	if strings.Contains(body, "/keys") || strings.Contains(body, "/auth") || !strings.Contains(body, "/access") || !strings.Contains(body, "/delete") {
		t.Errorf("user detail: %s", body)
	}
	if strings.Contains(body, testKi) {
		t.Error("user sees Ki")
	}
	body = do(e.h, request("GET", "/subscribers/440100123456789", e.owner, nil)).Body.String()
	if !strings.Contains(body, "/keys") || !strings.Contains(body, "/auth") || strings.Contains(body, testKi) {
		t.Errorf("admin detail: %s", body)
	}

	// 一般ユーザーは Ki / OPc を取得も変更もできない。
	for _, path := range []string{"/subscribers/440100123456789/keys", "/subscribers/440100123456789/auth"} {
		if w := do(e.h, hx(request("POST", path, e.user, url.Values{"sqn": {"000000000000"}}))); w.Code != http.StatusForbidden {
			t.Errorf("user %s: %d", path, w.Code)
		}
	}
	if slices.ContainsFunc(e.admin.calls, func(c string) bool { return strings.HasPrefix(c, "get-keys") }) {
		t.Error("user's request reached the keys api")
	}

	// 管理者は Ki / OPc を表示できる。操作者が管理API に渡る。
	w := do(e.h, hx(request("POST", "/subscribers/440100123456789/keys", e.owner, url.Values{})))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), testKi) || !strings.HasPrefix(w.Body.String(), `<div id="keys">`) {
		t.Errorf("keys: %d %s", w.Code, w.Body.String())
	}
	if got := e.admin.lastCall(); got != "get-keys root" {
		t.Errorf("operator: %q", got)
	}

	if w := do(e.h, request("GET", "/subscribers/440100000000000", e.user, nil)); w.Code != http.StatusNotFound ||
		!strings.Contains(w.Body.String(), "登録されていません") {
		t.Errorf("missing: %d", w.Code)
	}
	if w := do(e.h, request("GET", "/subscribers/abc", e.user, nil)); w.Code != http.StatusNotFound {
		t.Errorf("malformed: %d", w.Code)
	}
}

func TestSubscriberUpdate(t *testing.T) {
	e := newSubscriberEnv(t)
	a, b := e.seedClient(t, "radius-a"), e.seedClient(t, "radius-b")
	e.seed(t, "440100123456789", a)

	// 一般ユーザーは許可クライアントと平文HTTP を変更できる。
	w := do(e.h, hx(request("POST", "/subscribers/440100123456789/access", e.user, url.Values{"clients": {fmt.Sprint(b)}, "allow_plain": {"on"}})))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "設定を変更しました") || !strings.HasPrefix(w.Body.String(), `<div id="subscriber">`) {
		t.Errorf("access: %d %s", w.Code, w.Body.String())
	}
	if sub := e.admin.subs["440100123456789"]; !sub.AllowPlain || !slices.Equal(sub.AllowedClientIDs, []int64{b}) {
		t.Errorf("after access = %+v", sub)
	}
	if got := e.admin.lastCallOf("update-subscriber"); got != "update-subscriber bob" {
		t.Errorf("operator: %q", got)
	}
	// チェックを全て外すと、許可クライアントは空になる。
	do(e.h, hx(request("POST", "/subscribers/440100123456789/access", e.user, url.Values{})))
	if sub := e.admin.subs["440100123456789"]; sub.AllowPlain || len(sub.AllowedClientIDs) != 0 {
		t.Errorf("after clearing = %+v", sub)
	}

	// 管理者: 値が変わった項目だけを送る。
	cur := e.admin.subs["440100123456789"]
	form := url.Values{"ki": {""}, "opc": {""}, "sqn": {cur.SQN}, "amf": {cur.AMF}, "sqn_type": {cur.SQNType}}
	if w := do(e.h, hx(request("POST", "/subscribers/440100123456789/auth", e.owner, form))); !strings.Contains(w.Body.String(), "変更はありません") {
		t.Errorf("no change: %s", w.Body.String())
	}
	form = url.Values{"ki": {strings.Repeat("A", 32)}, "opc": {""}, "sqn": {"000000000040"}, "amf": {cur.AMF}, "sqn_type": {cur.SQNType}}
	w = do(e.h, hx(request("POST", "/subscribers/440100123456789/auth", e.owner, form)))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Ki、SQN を変更しました。") {
		t.Errorf("auth: %d %s", w.Code, w.Body.String())
	}
	if k := e.admin.keys["440100123456789"]; k.Ki != strings.Repeat("a", 32) || k.OPc != testOPc {
		t.Errorf("keys after auth = %+v", k)
	}
	form = url.Values{"ki": {"short"}, "sqn": {"zz"}, "amf": {"8000"}, "sqn_type": {"inc32"}}
	w = do(e.h, hx(request("POST", "/subscribers/440100123456789/auth", e.owner, form)))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "Ki は 16 進 32 桁") || !strings.Contains(w.Body.String(), "SQN は 16 進 12 桁") {
		t.Errorf("auth invalid: %d %s", w.Code, w.Body.String())
	}
}

func TestSubscriberDelete(t *testing.T) {
	e := newSubscriberEnv(t)
	e.seed(t, "440100123456789")

	w := do(e.h, hx(request("POST", "/subscribers/440100123456789/delete", e.user, url.Values{})))
	if w.Header().Get("HX-Redirect") != "/subscribers?deleted=440100123456789" {
		t.Errorf("delete: %d %v", w.Code, w.Header())
	}
	if got := e.admin.lastCall(); got != "delete-subscriber bob" {
		t.Errorf("operator: %q", got)
	}
	if body := do(e.h, request("GET", "/subscribers?deleted=440100123456789", e.user, nil)).Body.String(); !strings.Contains(body, "加入者 440100123456789 を削除しました。") {
		t.Error("deleted message")
	}
	// htmx を使わない送信でも一覧へ移動する。
	e.seed(t, "440100123456780")
	w = do(e.h, request("POST", "/subscribers/440100123456780/delete", e.user, url.Values{}))
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/subscribers?deleted=440100123456780" {
		t.Errorf("plain delete: %d %q", w.Code, w.Header().Get("Location"))
	}
}

func TestSubscriberAdminUnavailable(t *testing.T) {
	e := newSubscriberEnv(t)
	e.admin.err = fmt.Errorf("wrap: %w", &net.OpError{Op: "dial", Err: fmt.Errorf("connection refused")})
	w := do(e.h, request("GET", "/subscribers", e.user, nil))
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "管理API に接続できません") {
		t.Errorf("list: %d %s", w.Code, w.Body.String())
	}
}
