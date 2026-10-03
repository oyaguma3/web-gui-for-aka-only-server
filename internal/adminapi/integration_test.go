package adminapi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oyaguma3/web-gui-for-aka-only-server/internal/certs"
)

// 実際の aka-only-server の管理API を相手にした契約テスト。
// 次の環境変数を指定したときだけ実行する。
//
//	WEBGUI_TEST_ADMIN_URL          管理API のベース URL（例: https://100.64.0.1:9443/admin/v1）
//	WEBGUI_TEST_ADMIN_CLIENT_CERT  BFF のクライアント証明書と秘密鍵の PEM
//	WEBGUI_TEST_ADMIN_SERVER_CERT  管理API のサーバー証明書の PEM
//	WEBGUI_TEST_ADMIN_AV_CERT=1    AV用サーバー証明書の差し替えと再生成も試す（稼働中の AVクライアントに影響する）
//
// テスト用の加入者（IMSI 00101 で始まるテスト用の番号）と AVクライアントを作り、終わったら削除する。
func newIntegrationClient(t *testing.T) *Client {
	t.Helper()
	url := os.Getenv("WEBGUI_TEST_ADMIN_URL")
	if url == "" {
		t.Skip("WEBGUI_TEST_ADMIN_URL is not set")
	}
	c, err := New(Options{
		BaseURL:        url,
		ClientCertFile: os.Getenv("WEBGUI_TEST_ADMIN_CLIENT_CERT"),
		ServerCertFile: os.Getenv("WEBGUI_TEST_ADMIN_SERVER_CERT"),
		Log:            discard,
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// cleanupSubscriber は、テストの終わりに加入者を削除する（削除済みなら何もしない）。
// t.Context() は後片付けの前にキャンセルされるので、ここでは使わない。
func cleanupSubscriber(t *testing.T, c *Client, imsi string) {
	t.Cleanup(func() {
		err := c.DeleteSubscriber(WithOperator(context.Background(), "it-cleanup"), imsi)
		if err != nil && CauseOf(err) != CauseUserNotFound {
			t.Errorf("cleanup subscriber %s: %v", imsi, err)
		}
	})
}

// findAudit は、監査ログの新しい方から action と target が一致するものを探す。
func findAudit(t *testing.T, c *Client, action, target string) AuditLogEntry {
	t.Helper()
	l, err := c.ListAuditLogs(t.Context(), ListAuditLogsParams{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(l.Items, func(e AuditLogEntry) bool { return e.Action == action && e.Target == target })
	if i < 0 {
		t.Fatalf("audit %s %s not found", action, target)
	}
	return l.Items[i]
}

func TestIntegrationSubscribers(t *testing.T) {
	c := newIntegrationClient(t)
	ctx := WithOperator(t.Context(), "it-alice")
	imsi := fmt.Sprintf("00101%010d", time.Now().UnixNano()%1e10)
	cleanupSubscriber(t, c, imsi)

	before, err := c.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	// 登録（既定値に任せる項目は送らない）。
	created, err := c.CreateSubscriber(ctx, SubscriberCreate{
		IMSI: imsi, Ki: "465B5CE8B199B49FAA5F0A2EE238A6BC", OPc: "cd63cb71954a9f4e48a5994e37a02baf",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.SQN != "000000000000" || created.AMF != "8000" || created.SQNType != SQNTypeInc32 ||
		created.AllowPlain || len(created.AllowedClientIDs) != 0 || created.CreatedAt.IsZero() {
		t.Errorf("created = %+v", created)
	}
	if e := findAudit(t, c, AuditSubscriberCreate, imsi); e.Operator != "it-alice" || e.MgmtClient == "" {
		t.Errorf("audit create = %+v", e)
	}

	// 同じ IMSI はもう登録できない。
	_, err = c.CreateSubscriber(ctx, SubscriberCreate{IMSI: imsi, Ki: strings.Repeat("0", 32), OPc: strings.Repeat("0", 32)})
	if CauseOf(err) != CauseSubscriberExists {
		t.Errorf("duplicate: err = %v", err)
	}

	// 入力の誤りは項目つきで返る。
	_, err = c.CreateSubscriber(ctx, SubscriberCreate{IMSI: "00101000000000x", Ki: "zz", OPc: strings.Repeat("0", 32)})
	if apiErr, ok := errors.AsType[*Error](err); !ok || apiErr.Status != 400 || len(apiErr.Problem.InvalidParams) == 0 {
		t.Errorf("invalid: err = %v", err)
	}

	got, err := c.GetSubscriber(t.Context(), imsi)
	if err != nil || got.IMSI != imsi {
		t.Fatalf("get: %+v, %v", got, err)
	}

	// 一覧（前方一致）と件数。
	list, err := c.ListSubscribers(t.Context(), ListSubscribersParams{Prefix: imsi, Limit: 10})
	if err != nil || list.Total != 1 || len(list.Items) != 1 || list.Items[0].IMSI != imsi {
		t.Errorf("list: %+v, %v", list, err)
	}
	if after, err := c.Status(t.Context()); err != nil || after.SubscriberCount != before.SubscriberCount+1 {
		t.Errorf("status count: before %d, after %+v, %v", before.SubscriberCount, after, err)
	}

	// Ki / OPc は小文字で返り、取得は監査ログに残る。
	keys, err := c.GetSubscriberKeys(ctx, imsi)
	if err != nil || keys.Ki != "465b5ce8b199b49faa5f0a2ee238a6bc" || keys.OPc != "cd63cb71954a9f4e48a5994e37a02baf" {
		t.Errorf("keys: %+v, %v", keys, err)
	}
	if e := findAudit(t, c, AuditSubscriberKeysRead, imsi); e.Operator != "it-alice" {
		t.Errorf("audit keys = %+v", e)
	}

	// 変更（Merge Patch）。
	updated, err := c.UpdateSubscriber(WithOperator(t.Context(), "it-bob"), imsi, SubscriberUpdate{
		SQN: new("00000000001f"), SQNType: new(SQNTypeInc33), AllowPlain: new(true),
	})
	if err != nil || updated.SQN != "00000000001f" || updated.SQNType != SQNTypeInc33 || !updated.AllowPlain || updated.AMF != "8000" {
		t.Errorf("update: %+v, %v", updated, err)
	}
	if e := findAudit(t, c, AuditSubscriberUpdate, imsi); e.Operator != "it-bob" || e.Detail["sqn"] == nil {
		t.Errorf("audit update = %+v", e)
	}

	// 削除。
	if err := c.DeleteSubscriber(ctx, imsi); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetSubscriber(t.Context(), imsi); CauseOf(err) != CauseUserNotFound {
		t.Errorf("get after delete: err = %v", err)
	}
	if e := findAudit(t, c, AuditSubscriberDelete, imsi); e.Operator != "it-alice" || e.Detail["sqn"] != "00000000001f" {
		t.Errorf("audit delete = %+v", e)
	}
}

func TestIntegrationAVClients(t *testing.T) {
	c := newIntegrationClient(t)
	ctx := WithOperator(t.Context(), "it-alice")

	certPEM, _, err := certs.SelfSigned("it-av-client", nil, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	created, err := c.CreateAVClient(ctx, AVClientCreate{Name: "it-client", CertPEM: string(certPEM), Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		err := c.DeleteAVClient(WithOperator(context.Background(), "it-cleanup"), created.ID)
		if err != nil && CauseOf(err) != CauseClientNotFound {
			t.Errorf("cleanup client %d: %v", created.ID, err)
		}
	})
	if created.ID == 0 || !created.Enabled || created.Subject == "" || len(created.FingerprintSHA256) != 64 {
		t.Errorf("created = %+v", created)
	}

	// 同じ証明書は二重に登録できない。
	if _, err := c.CreateAVClient(ctx, AVClientCreate{Name: "dup", CertPEM: string(certPEM), Enabled: true}); CauseOf(err) != CauseCertRegistered {
		t.Errorf("duplicate: err = %v", err)
	}
	// 証明書でないものは INVALID_CERTIFICATE。
	if _, err := c.CreateAVClient(ctx, AVClientCreate{Name: "bad", CertPEM: "not a pem", Enabled: true}); CauseOf(err) != CauseInvalidCertificate {
		t.Errorf("invalid cert: err = %v", err)
	}

	list, err := c.ListAVClients(t.Context())
	if err != nil || !slices.ContainsFunc(list, func(a AVClient) bool { return a.ID == created.ID }) {
		t.Errorf("list: %v, %v", list, err)
	}

	updated, err := c.UpdateAVClient(ctx, created.ID, AVClientUpdate{Enabled: new(false), NetworkName: new("WLAN")})
	if err != nil || updated.Enabled || updated.NetworkName != "WLAN" || updated.Name != "it-client" {
		t.Errorf("update: %+v, %v", updated, err)
	}

	newPEM, _, err := certs.SelfSigned("it-av-client-2", nil, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	replaced, err := c.ReplaceAVClientCertificate(ctx, created.ID, string(newPEM))
	if err != nil || replaced.ID != created.ID || replaced.FingerprintSHA256 == created.FingerprintSHA256 {
		t.Errorf("replace: %+v, %v", replaced, err)
	}

	// 許可クライアントに入れた加入者を作り、クライアントの削除で外れることを確かめる。
	imsi := fmt.Sprintf("00101%010d", time.Now().UnixNano()%1e10)
	cleanupSubscriber(t, c, imsi)
	if _, err := c.CreateSubscriber(ctx, SubscriberCreate{
		IMSI: imsi, Ki: strings.Repeat("1", 32), OPc: strings.Repeat("2", 32), AllowedClientIDs: []int64{created.ID},
	}); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteAVClient(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetAVClient(t.Context(), created.ID); CauseOf(err) != CauseClientNotFound {
		t.Errorf("get after delete: err = %v", err)
	}
	if sub, err := c.GetSubscriber(t.Context(), imsi); err != nil || len(sub.AllowedClientIDs) != 0 {
		t.Errorf("subscriber after client delete: %+v, %v", sub, err)
	}
	if e := findAudit(t, c, AuditClientDelete, fmt.Sprint(created.ID)); e.Operator != "it-alice" {
		t.Errorf("audit client delete = %+v", e)
	}
}

func TestIntegrationLogs(t *testing.T) {
	c := newIntegrationClient(t)

	l, err := c.ListLogs(t.Context(), ListLogsParams{Limit: 5})
	if err != nil || l.BootID == "" || len(l.Items) == 0 || len(l.Items) > 5 {
		t.Fatalf("logs: %+v, %v", l, err)
	}
	st, err := c.Status(t.Context())
	if err != nil || st.BootID != l.BootID {
		t.Errorf("bootId: status %q, logs %q, %v", st.BootID, l.BootID, err)
	}
	// 続きの取得。lastSeq より後はまだない（または少しだけある）。
	next, err := c.ListLogs(t.Context(), ListLogsParams{After: new(l.LastSeq)})
	if err != nil || next.LastSeq < l.LastSeq {
		t.Errorf("logs after: %+v, %v", next, err)
	}
	for _, e := range next.Items {
		if e.Seq <= l.LastSeq {
			t.Errorf("seq %d is not after %d", e.Seq, l.LastSeq)
		}
	}

	// 監査ログのページング。
	page, err := c.ListAuditLogs(t.Context(), ListAuditLogsParams{Limit: 1})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("audit: %+v, %v", page, err)
	}
	if page.NextBefore != "" {
		older, err := c.ListAuditLogs(t.Context(), ListAuditLogsParams{Before: page.NextBefore, Limit: 1})
		if err != nil || len(older.Items) != 1 || older.Items[0].ID == page.Items[0].ID {
			t.Errorf("audit older: %+v, %v", older, err)
		}
	}
}

func TestIntegrationAVServerCertificate(t *testing.T) {
	c := newIntegrationClient(t)
	cur, err := c.AVServerCertificate(t.Context())
	if err != nil || cur.CertPEM == "" || cur.Source == "" {
		t.Fatalf("get: %+v, %v", cur, err)
	}
	if os.Getenv("WEBGUI_TEST_ADMIN_AV_CERT") != "1" {
		t.Skip("WEBGUI_TEST_ADMIN_AV_CERT is not 1; skip replacing the av server certificate")
	}
	ctx := WithOperator(t.Context(), "it-alice")

	certPEM, keyPEM, err := certs.SelfSigned("it-av-server", []string{"aka-only-server"}, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	replaced, err := c.ReplaceAVServerCertificate(ctx, string(certPEM), string(keyPEM))
	if err != nil || replaced.Source != CertSourceUploaded || replaced.Subject == cur.Subject {
		t.Errorf("replace: %+v, %v", replaced, err)
	}
	// 秘密鍵が対応しないものは受け付けない。
	_, otherKey, err := certs.SelfSigned("x", nil, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ReplaceAVServerCertificate(ctx, string(certPEM), string(otherKey)); CauseOf(err) != CauseInvalidCertificate {
		t.Errorf("mismatched key: err = %v", err)
	}

	reset, err := c.ResetAVServerCertificate(ctx)
	if err != nil || reset.Source != CertSourceSelfSigned || reset.FingerprintSHA256 == replaced.FingerprintSHA256 {
		t.Errorf("reset: %+v, %v", reset, err)
	}
	if e := findAudit(t, c, AuditAVServerCertReset, "av-server-certificate"); e.Operator != "it-alice" {
		t.Errorf("audit reset = %+v", e)
	}
}
