package web

import (
	"cmp"
	"encoding/json/v2"
	"net/http"
	"time"

	"github.com/oyaguma3/web-gui-for-aka-only-server/internal/adminapi"
)

// auditPerPage は監査ログの 1 回に読み込む件数。
const auditPerPage = 50

// auditActionLabels は監査ログの操作の名前。
var auditActionLabels = map[string]string{
	// aka-only-server
	adminapi.AuditSubscriberCreate:    "加入者の登録",
	adminapi.AuditSubscriberUpdate:    "加入者の変更",
	adminapi.AuditSubscriberDelete:    "加入者の削除",
	adminapi.AuditSubscriberKeysRead:  "Ki / OPc の表示",
	adminapi.AuditClientCreate:        "AVクライアントの登録",
	adminapi.AuditClientUpdate:        "AVクライアントの変更",
	adminapi.AuditClientCertReplace:   "AVクライアントの証明書の差し替え",
	adminapi.AuditClientDelete:        "AVクライアントの削除",
	adminapi.AuditAVServerCertReplace: "AV用サーバー証明書の差し替え",
	adminapi.AuditAVServerCertReset:   "AV用サーバー証明書の作り直し",
	// BFF
	"login.success":           "ログイン",
	"login.failure":           "ログインの失敗",
	"account.create":          "アカウントの作成",
	"account.delete":          "アカウントの削除",
	"account.password.reset":  "パスワードの再設定",
	"account.password.change": "パスワードの変更",
}

// auditRow は監査ログの 1 行。aka-only-server と BFF のどちらの監査ログもこの形にして出す。
type auditRow struct {
	ID       string
	Time     time.Time
	Operator string
	// Via は aka-only-server の監査ログでは管理クライアント名、BFF の監査ログでは操作元のアドレス。
	Via    string
	Action string
	Target string
	Detail string
}

type auditData struct {
	Source string // server / bff
	Rows   []auditRow
	Next   string
	Error  string
	// More は「さらに古いものを表示」で続きを読み込んだ応答か。
	More bool
}

// audit は監査ログの画面。管理者だけが使える。
// source=server は aka-only-server の監査ログ（管理API の操作）、source=bff は BFF の監査ログ（ログインとアカウント）。
func (h *Handler) audit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	d := auditData{Source: "server", More: q.Get("before") != ""}
	if q.Get("source") == "bff" {
		d.Source = "bff"
	}
	status := http.StatusOK
	if d.Source == "server" {
		l, err := h.admin.ListAuditLogs(r.Context(), adminapi.ListAuditLogsParams{Before: q.Get("before"), Limit: auditPerPage})
		if err != nil {
			status, d.Error = apiErrorMessage(err, "")
			h.log.Warn("list audit logs", "error", err)
		}
		for _, e := range l.Items {
			d.Rows = append(d.Rows, auditRow{
				ID: e.ID, Time: e.Time, Operator: cmp.Or(e.Operator, "（不明）"), Via: e.MgmtClient,
				Action: e.Action, Target: e.Target, Detail: compactJSON(e.Detail),
			})
		}
		d.Next = l.NextBefore
	} else {
		me, _ := accountFrom(r.Context())
		entries, next, err := h.auth.ListAudit(r.Context(), me, q.Get("before"), auditPerPage)
		if err != nil {
			status, d.Error = http.StatusInternalServerError, "BFF の監査ログを取得できませんでした。"
			h.log.Error("list bff audit", "error", err)
		}
		for _, e := range entries {
			d.Rows = append(d.Rows, auditRow{
				ID: e.ID, Time: e.Time, Operator: e.Actor, Via: e.Remote,
				Action: e.Action, Target: e.Target, Detail: e.Detail,
			})
		}
		d.Next = next
	}
	if d.More && r.Header.Get("HX-Request") == "true" {
		h.renderBlock(w, r, status, "audit", "audit-more", d)
		return
	}
	h.render(w, r, status, "audit", "監査ログ", d)
}

// compactJSON は監査ログの変更内容を 1 行の JSON にする。
func compactJSON(v map[string]any) string {
	if len(v) == 0 {
		return ""
	}
	b, err := json.Marshal(v, json.Deterministic(true))
	if err != nil {
		return ""
	}
	return string(b)
}
