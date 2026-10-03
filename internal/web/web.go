// Package web はブラウザ向けの画面（HTML）と静的ファイルを返す。
//
// 同梱しているサードパーティのファイル（static/）:
//   - htmx.min.js: htmx 2.0.11（https://htmx.org、0BSD）
//   - pico.min.css: Pico CSS 2.1.1（https://picocss.com、MIT。著作権表示はファイル先頭）
//
// 更新するときは npm の配布物（jsdelivr）から同じファイル名で置き換え、上の版数も直す。
package web

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/oyaguma3/web-gui-for-aka-only-server/internal/adminapi"
	"github.com/oyaguma3/web-gui-for-aka-only-server/internal/auth"
	"github.com/oyaguma3/web-gui-for-aka-only-server/internal/store"
)

//go:embed templates static
var assets embed.FS

// AdminAPI は画面が使う管理API の操作。*adminapi.Client が満たす。
type AdminAPI interface {
	Status(ctx context.Context) (adminapi.Status, error)

	ListSubscribers(ctx context.Context, p adminapi.ListSubscribersParams) (adminapi.SubscriberList, error)
	CreateSubscriber(ctx context.Context, s adminapi.SubscriberCreate) (adminapi.Subscriber, error)
	GetSubscriber(ctx context.Context, imsi string) (adminapi.Subscriber, error)
	UpdateSubscriber(ctx context.Context, imsi string, u adminapi.SubscriberUpdate) (adminapi.Subscriber, error)
	DeleteSubscriber(ctx context.Context, imsi string) error
	GetSubscriberKeys(ctx context.Context, imsi string) (adminapi.SubscriberKeys, error)

	ListAVClients(ctx context.Context) ([]adminapi.AVClient, error)
	CreateAVClient(ctx context.Context, a adminapi.AVClientCreate) (adminapi.AVClient, error)
	GetAVClient(ctx context.Context, id int64) (adminapi.AVClient, error)
	UpdateAVClient(ctx context.Context, id int64, u adminapi.AVClientUpdate) (adminapi.AVClient, error)
	DeleteAVClient(ctx context.Context, id int64) error
	ReplaceAVClientCertificate(ctx context.Context, id int64, certPEM string) (adminapi.AVClient, error)

	AVServerCertificate(ctx context.Context) (adminapi.ServerCertificate, error)
	ReplaceAVServerCertificate(ctx context.Context, certPEM, keyPEM string) (adminapi.ServerCertificate, error)
	ResetAVServerCertificate(ctx context.Context) (adminapi.ServerCertificate, error)

	ListLogs(ctx context.Context, p adminapi.ListLogsParams) (adminapi.LogList, error)
	ListAuditLogs(ctx context.Context, p adminapi.ListAuditLogsParams) (adminapi.AuditLogList, error)
}

// AuthService はログイン、セッション、アカウントの操作。*auth.Service が満たす。
type AuthService interface {
	Login(ctx context.Context, id, password string) (string, auth.Account, error)
	Authenticate(ctx context.Context, token string) (auth.Account, error)
	AuthenticatePassive(ctx context.Context, token string) (auth.Account, error)
	ListAudit(ctx context.Context, actor auth.Account, before string, limit int) ([]store.AuditEntry, string, error)
	Logout(ctx context.Context, token string) error
	ListAccounts(ctx context.Context, actor auth.Account) ([]auth.Account, error)
	CreateAccount(ctx context.Context, actor auth.Account, id string, role auth.Role, password string) error
	DeleteAccount(ctx context.Context, actor auth.Account, id string) error
	ResetPassword(ctx context.Context, actor auth.Account, id, password string) error
	ChangePassword(ctx context.Context, actor auth.Account, token, current, password string) (string, error)
}

// Handler は画面のハンドラー。
type Handler struct {
	log     *slog.Logger
	version string
	admin   AdminAPI
	auth    AuthService
	pages   pages
	static  *staticFiles
}

// Options は Handler の設定。
type Options struct {
	Log *slog.Logger
	// Version は画面のフッターに出すバージョン。
	Version string
	Admin   AdminAPI
	Auth    AuthService
}

// New は Handler を作る。テンプレートと静的ファイルはここで読み込む。
func New(opts Options) (*Handler, error) {
	h := &Handler{log: opts.Log, version: opts.Version, admin: opts.Admin, auth: opts.Auth}
	var err error
	if h.pages, err = parsePages(); err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	if h.static, err = newStaticFiles(http.HandlerFunc(h.notFound)); err != nil {
		return nil, fmt.Errorf("load static files: %w", err)
	}
	return h, nil
}

// Routes はルーティングとミドルウェアを組み立てた http.Handler を返す。
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", h.static)

	mux.HandleFunc("GET /login", h.loginPage)
	mux.HandleFunc("POST /login", h.login)
	mux.HandleFunc("POST /logout", h.logout)
	mux.Handle("GET /password", h.authedForPasswordChange(h.passwordPage))
	mux.Handle("POST /password", h.authedForPasswordChange(h.changePassword))

	mux.Handle("GET /{$}", h.authed(h.dashboard))

	mux.Handle("GET /subscribers", h.authed(h.subscribers))
	mux.Handle("GET /subscribers/new", h.authed(h.subscriberNew))
	mux.Handle("POST /subscribers", h.authed(h.subscriberCreate))
	mux.Handle("GET /subscribers/{imsi}", h.authed(h.subscriber))
	mux.Handle("POST /subscribers/{imsi}/access", h.authed(h.subscriberAccess))
	mux.Handle("POST /subscribers/{imsi}/auth", h.adminOnly(h.subscriberAuth))
	mux.Handle("POST /subscribers/{imsi}/keys", h.adminOnly(h.subscriberKeys))
	mux.Handle("POST /subscribers/{imsi}/delete", h.authed(h.subscriberDelete))

	mux.Handle("GET /av-clients", h.authed(h.avClients))
	mux.Handle("POST /av-clients", h.adminOnly(h.avClientCreate))
	mux.Handle("GET /av-clients/{id}", h.authed(h.avClient))
	mux.Handle("POST /av-clients/{id}/update", h.adminOnly(h.avClientUpdate))
	mux.Handle("POST /av-clients/{id}/certificate", h.adminOnly(h.avClientCertificate))
	mux.Handle("POST /av-clients/{id}/delete", h.adminOnly(h.avClientDelete))

	mux.Handle("GET /av-server-certificate", h.authed(h.serverCert))
	mux.Handle("GET /av-server-certificate.pem", h.authed(h.serverCertPEM))
	mux.Handle("POST /av-server-certificate", h.adminOnly(h.serverCertReplace))
	mux.Handle("POST /av-server-certificate/reset", h.adminOnly(h.serverCertReset))

	mux.Handle("GET /logs", h.authed(h.logs))
	mux.Handle("GET /logs/tail", h.authedPassive(h.logsTail))
	mux.Handle("GET /audit", h.adminOnly(h.audit))

	mux.Handle("GET /accounts", h.adminOnly(h.accountsPage))
	mux.Handle("POST /accounts", h.adminOnly(h.createAccount))
	mux.Handle("POST /accounts/{id}/delete", h.adminOnly(h.deleteAccount))
	mux.Handle("POST /accounts/{id}/password", h.adminOnly(h.resetPassword))

	mux.HandleFunc("/", h.notFound)

	cop := http.NewCrossOriginProtection()
	cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.log.Warn("cross-origin request rejected",
			"method", r.Method, "path", r.URL.Path, "origin", r.Header.Get("Origin"))
		h.renderError(w, r, http.StatusForbidden, "別のサイトからの操作は受け付けません。")
	}))

	var handler http.Handler = mux
	handler = cop.Handler(handler)
	handler = withRemote(handler)
	handler = securityHeaders(handler)
	handler = h.accessLog(handler)
	return handler
}

// dashboardLogs はダッシュボードに出す直近のログの件数。
const dashboardLogs = 10

// dashboardData はダッシュボードに渡す値。
type dashboardData struct {
	Status adminapi.Status
	// Logs は直近のログ（新しい順に並べるのはテンプレートで行う）。
	Logs []adminapi.LogEntry
	// ExpiringClients は証明書が期限切れか、30 日以内に切れる AVクライアント。
	ExpiringClients []adminapi.AVClient
	// AdminError は管理API から状態を取得できなかった場合の説明。
	AdminError string
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	var d dashboardData
	st, err := h.admin.Status(r.Context())
	if err != nil {
		h.log.Warn("get admin api status", "error", err)
		d.AdminError = adminErrorMessage(err)
		h.render(w, r, http.StatusOK, "dashboard", "ダッシュボード", d)
		return
	}
	d.Status = st
	// 状態が取れていれば、ログとクライアントは取れなくても表示を続ける。
	if l, err := h.admin.ListLogs(r.Context(), adminapi.ListLogsParams{Limit: dashboardLogs}); err == nil {
		d.Logs = l.Items
	} else {
		h.log.Warn("list logs", "error", err)
	}
	if clients, err := h.admin.ListAVClients(r.Context()); err == nil {
		for _, c := range clients {
			if time.Until(c.NotAfter) <= 30*24*time.Hour {
				d.ExpiringClients = append(d.ExpiringClients, c)
			}
		}
	} else {
		h.log.Warn("list av clients", "error", err)
	}
	h.render(w, r, http.StatusOK, "dashboard", "ダッシュボード", d)
}

// adminErrorMessage は、管理API の呼び出しに失敗したときに画面に出す説明を返す。
func adminErrorMessage(err error) string {
	if adminapi.IsUnavailable(err) {
		if hint := adminapi.Diagnose(err); hint != "" {
			return hint
		}
		return "aka-only-server の管理API に接続できません。"
	}
	return "aka-only-server の管理API がエラーを返しました。"
}

func (h *Handler) notFound(w http.ResponseWriter, r *http.Request) {
	h.renderError(w, r, http.StatusNotFound, "ページが見つかりません。")
}
