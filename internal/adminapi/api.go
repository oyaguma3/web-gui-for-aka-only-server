package adminapi

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ---- 状態 ----

// Status はサーバーの状態。
type Status struct {
	Version                     string    `json:"version"`
	BootID                      string    `json:"bootId"`
	StartedAt                   time.Time `json:"startedAt"`
	SubscriberCount             int64     `json:"subscriberCount"`
	ClientCount                 int64     `json:"clientCount"`
	AVPlainEnabled              bool      `json:"avPlainEnabled"`
	AVServerCertificateNotAfter time.Time `json:"avServerCertificateNotAfter"`
}

// Status はサーバーの状態を取得する。
func (c *Client) Status(ctx context.Context) (Status, error) {
	return c.call[Status](ctx, request{method: http.MethodGet, path: []string{"status"}})
}

// ---- 加入者 ----

// SQN 増加タイプ。
const (
	SQNTypeInc1  = "inc1"
	SQNTypeInc32 = "inc32"
	SQNTypeInc33 = "inc33"
)

// Subscriber は加入者。Ki と OPc は含まない。
type Subscriber struct {
	IMSI             string    `json:"imsi"`
	SQN              string    `json:"sqn"`
	AMF              string    `json:"amf"`
	SQNType          string    `json:"sqnType"`
	AllowPlain       bool      `json:"allowPlain"`
	AllowedClientIDs []int64   `json:"allowedClientIds"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// SubscriberCreate は加入者の登録内容。SQN、AMF、SQNType は空なら管理API の既定値になる。
type SubscriberCreate struct {
	IMSI             string  `json:"imsi"`
	Ki               string  `json:"ki"`
	OPc              string  `json:"opc"`
	SQN              string  `json:"sqn,omitempty"`
	AMF              string  `json:"amf,omitempty"`
	SQNType          string  `json:"sqnType,omitempty"`
	AllowPlain       bool    `json:"allowPlain"`
	AllowedClientIDs []int64 `json:"allowedClientIds"`
}

// SubscriberUpdate は加入者の変更内容（JSON Merge Patch）。nil の項目は変更しない。
// AllowedClientIDs を指定すると集合全体を置き換える（空のスライスなら全て外す）。
type SubscriberUpdate struct {
	Ki               *string  `json:"ki,omitzero"`
	OPc              *string  `json:"opc,omitzero"`
	SQN              *string  `json:"sqn,omitzero"`
	AMF              *string  `json:"amf,omitzero"`
	SQNType          *string  `json:"sqnType,omitzero"`
	AllowPlain       *bool    `json:"allowPlain,omitzero"`
	AllowedClientIDs *[]int64 `json:"allowedClientIds,omitzero"`
}

// SubscriberKeys は加入者の Ki と OPc。
type SubscriberKeys struct {
	Ki  string `json:"ki"`
	OPc string `json:"opc"`
}

// SubscriberList は加入者の一覧の 1 ページ。
type SubscriberList struct {
	Items []Subscriber `json:"items"`
	// Total は Prefix に一致する加入者の総数。
	Total int64 `json:"total"`
	// NextCursor は次のページがある場合だけ入る。
	NextCursor string `json:"nextCursor"`
}

// ListSubscribersParams は加入者の一覧の条件。ゼロ値の項目は指定しない。
type ListSubscribersParams struct {
	// Prefix は IMSI の前方一致条件。
	Prefix string
	// Cursor は前のページの NextCursor。
	Cursor string
	// Limit は 1 ページの件数（1〜500、省略時は 50）。
	Limit int
}

// ListSubscribers は加入者の一覧を IMSI の辞書順で取得する。
func (c *Client) ListSubscribers(ctx context.Context, p ListSubscribersParams) (SubscriberList, error) {
	q := url.Values{}
	setString(q, "prefix", p.Prefix)
	setString(q, "cursor", p.Cursor)
	setInt(q, "limit", int64(p.Limit))
	return c.call[SubscriberList](ctx, request{method: http.MethodGet, path: []string{"subscribers"}, query: q})
}

// CreateSubscriber は加入者を登録する。操作者が必要。
func (c *Client) CreateSubscriber(ctx context.Context, s SubscriberCreate) (Subscriber, error) {
	return c.call[Subscriber](ctx, request{
		method: http.MethodPost, path: []string{"subscribers"}, body: s, needOperator: true,
	})
}

// GetSubscriber は加入者を取得する。
func (c *Client) GetSubscriber(ctx context.Context, imsi string) (Subscriber, error) {
	return c.call[Subscriber](ctx, request{method: http.MethodGet, path: []string{"subscribers", imsi}})
}

// UpdateSubscriber は加入者を変更する。操作者が必要。
func (c *Client) UpdateSubscriber(ctx context.Context, imsi string, u SubscriberUpdate) (Subscriber, error) {
	return c.call[Subscriber](ctx, request{
		method: http.MethodPatch, path: []string{"subscribers", imsi}, body: u,
		contentType: "application/merge-patch+json", needOperator: true,
	})
}

// DeleteSubscriber は加入者を削除する。操作者が必要。
func (c *Client) DeleteSubscriber(ctx context.Context, imsi string) error {
	_, err := c.call[struct{}](ctx, request{
		method: http.MethodDelete, path: []string{"subscribers", imsi}, needOperator: true,
	})
	return err
}

// GetSubscriberKeys は加入者の Ki と OPc を取得する。管理API の監査ログに残る。操作者が必要。
func (c *Client) GetSubscriberKeys(ctx context.Context, imsi string) (SubscriberKeys, error) {
	return c.call[SubscriberKeys](ctx, request{
		method: http.MethodGet, path: []string{"subscribers", imsi, "keys"}, needOperator: true,
	})
}

// ---- AVクライアント ----

// AVClient は AVクライアント（認証ベクターAPI のクライアント）。
type AVClient struct {
	ID                int64     `json:"id"`
	Name              string    `json:"name"`
	CertPEM           string    `json:"certPem"`
	FingerprintSHA256 string    `json:"fingerprintSha256"`
	Subject           string    `json:"subject"`
	NotBefore         time.Time `json:"notBefore"`
	NotAfter          time.Time `json:"notAfter"`
	Enabled           bool      `json:"enabled"`
	NetworkName       string    `json:"networkName"`
	CreatedAt         time.Time `json:"createdAt"`
}

// AVClientCreate は AVクライアントの登録内容。
type AVClientCreate struct {
	Name    string `json:"name"`
	CertPEM string `json:"certPem"`
	Enabled bool   `json:"enabled"`
	// NetworkName は EAP-AKA' の鍵導出に使う Network Name。空なら既定値（WLAN）。
	NetworkName string `json:"networkName"`
}

// AVClientUpdate は AVクライアントの変更内容（JSON Merge Patch）。nil の項目は変更しない。
type AVClientUpdate struct {
	Name        *string `json:"name,omitzero"`
	Enabled     *bool   `json:"enabled,omitzero"`
	NetworkName *string `json:"networkName,omitzero"`
}

type avClientList struct {
	Items []AVClient `json:"items"`
}

// ListAVClients は AVクライアントの一覧をクライアントID の昇順で取得する。
func (c *Client) ListAVClients(ctx context.Context) ([]AVClient, error) {
	l, err := c.call[avClientList](ctx, request{method: http.MethodGet, path: []string{"clients"}})
	return l.Items, err
}

// CreateAVClient は AVクライアントを登録する。操作者が必要。
func (c *Client) CreateAVClient(ctx context.Context, a AVClientCreate) (AVClient, error) {
	return c.call[AVClient](ctx, request{
		method: http.MethodPost, path: []string{"clients"}, body: a, needOperator: true,
	})
}

// GetAVClient は AVクライアントを取得する。
func (c *Client) GetAVClient(ctx context.Context, id int64) (AVClient, error) {
	return c.call[AVClient](ctx, request{method: http.MethodGet, path: []string{"clients", formatID(id)}})
}

// UpdateAVClient は AVクライアントを変更する。操作者が必要。
func (c *Client) UpdateAVClient(ctx context.Context, id int64, u AVClientUpdate) (AVClient, error) {
	return c.call[AVClient](ctx, request{
		method: http.MethodPatch, path: []string{"clients", formatID(id)}, body: u,
		contentType: "application/merge-patch+json", needOperator: true,
	})
}

// DeleteAVClient は AVクライアントを削除する。操作者が必要。
func (c *Client) DeleteAVClient(ctx context.Context, id int64) error {
	_, err := c.call[struct{}](ctx, request{
		method: http.MethodDelete, path: []string{"clients", formatID(id)}, needOperator: true,
	})
	return err
}

// ReplaceAVClientCertificate は AVクライアントの証明書を差し替える。操作者が必要。
func (c *Client) ReplaceAVClientCertificate(ctx context.Context, id int64, certPEM string) (AVClient, error) {
	return c.call[AVClient](ctx, request{
		method: http.MethodPut, path: []string{"clients", formatID(id), "certificate"},
		body: struct {
			CertPEM string `json:"certPem"`
		}{certPEM},
		needOperator: true,
	})
}

// ---- AV用サーバー証明書 ----

// 証明書の出所。
const (
	CertSourceSelfSigned = "self-signed"
	CertSourceUploaded   = "uploaded"
)

// ServerCertificate は AV用サーバー証明書。秘密鍵は含まない。
type ServerCertificate struct {
	CertPEM           string    `json:"certPem"`
	Source            string    `json:"source"`
	FingerprintSHA256 string    `json:"fingerprintSha256"`
	Subject           string    `json:"subject"`
	NotBefore         time.Time `json:"notBefore"`
	NotAfter          time.Time `json:"notAfter"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

// AVServerCertificate は現在の AV用サーバー証明書を取得する。
func (c *Client) AVServerCertificate(ctx context.Context) (ServerCertificate, error) {
	return c.call[ServerCertificate](ctx, request{method: http.MethodGet, path: []string{"av-server-certificate"}})
}

// ReplaceAVServerCertificate は AV用サーバー証明書を持ち込みの証明書と秘密鍵に差し替える。操作者が必要。
func (c *Client) ReplaceAVServerCertificate(ctx context.Context, certPEM, keyPEM string) (ServerCertificate, error) {
	return c.call[ServerCertificate](ctx, request{
		method: http.MethodPut, path: []string{"av-server-certificate"},
		body: struct {
			CertPEM string `json:"certPem"`
			KeyPEM  string `json:"keyPem"`
		}{certPEM, keyPEM},
		needOperator: true,
	})
}

// ResetAVServerCertificate は AV用サーバー証明書を捨て、自己署名を再生成して置き換える。操作者が必要。
func (c *Client) ResetAVServerCertificate(ctx context.Context) (ServerCertificate, error) {
	return c.call[ServerCertificate](ctx, request{
		method: http.MethodDelete, path: []string{"av-server-certificate"}, needOperator: true,
	})
}

// ---- ログ ----

// LogEntry はサーバーログの 1 件。
type LogEntry struct {
	Seq   int64          `json:"seq"`
	Time  time.Time      `json:"time"`
	Level string         `json:"level"`
	Msg   string         `json:"msg"`
	Attrs map[string]any `json:"attrs"`
}

// LogList はサーバーログ。
type LogList struct {
	// BootID が前回と変わっていたら、サーバーが再起動して番号が振り直されている。
	BootID string     `json:"bootId"`
	Items  []LogEntry `json:"items"`
	// LastSeq は返した最後のログの番号。続きは After に渡して取得する。
	LastSeq int64 `json:"lastSeq"`
}

// ListLogsParams はサーバーログの取得条件。
type ListLogsParams struct {
	// After を指定すると、その番号より後のログだけを古い順に返す。nil なら最新の Limit 件。
	After *int64
	// Limit は件数（1〜1000、省略時は 200）。
	Limit int
}

// ListLogs はサーバーログを取得する。
func (c *Client) ListLogs(ctx context.Context, p ListLogsParams) (LogList, error) {
	q := url.Values{}
	if p.After != nil {
		q.Set("after", strconv.FormatInt(*p.After, 10))
	}
	setInt(q, "limit", int64(p.Limit))
	return c.call[LogList](ctx, request{method: http.MethodGet, path: []string{"logs"}, query: q})
}

// 監査ログの操作の種類。
const (
	AuditSubscriberCreate    = "subscriber.create"
	AuditSubscriberUpdate    = "subscriber.update"
	AuditSubscriberDelete    = "subscriber.delete"
	AuditSubscriberKeysRead  = "subscriber.keys.read"
	AuditClientCreate        = "client.create"
	AuditClientUpdate        = "client.update"
	AuditClientCertReplace   = "client.certificate.replace"
	AuditClientDelete        = "client.delete"
	AuditAVServerCertReplace = "av-server-certificate.replace"
	AuditAVServerCertReset   = "av-server-certificate.reset"
)

// AuditLogEntry は管理API の監査ログの 1 件。
type AuditLogEntry struct {
	ID   string    `json:"id"`
	Time time.Time `json:"time"`
	// Operator は X-Operator-Id で渡した操作者。渡さなかった場合は空。
	Operator string `json:"operator"`
	// MgmtClient は管理クライアント（BFF）の識別名。
	MgmtClient string `json:"mgmtClient"`
	Action     string `json:"action"`
	Target     string `json:"target"`
	// TraceID は X-Trace-ID で渡した（または aka-only-server が採番した）トレースID。
	// 管理API 0.2.0 より前のサーバーや、それより前に記録されたエントリでは空。
	TraceID string         `json:"traceId"`
	Detail  map[string]any `json:"detail"`
}

// AuditLogList は監査ログ（新しい順）。
type AuditLogList struct {
	Items []AuditLogEntry `json:"items"`
	// NextBefore はさらに古いエントリがある場合だけ入る。
	NextBefore string `json:"nextBefore"`
}

// ListAuditLogsParams は監査ログの取得条件。ゼロ値の項目は指定しない。
type ListAuditLogsParams struct {
	// Before はこのエントリID より古いものを返す。
	Before string
	// Limit は件数（1〜500、省略時は 100）。
	Limit int
}

// ListAuditLogs は監査ログを新しい順に取得する。
func (c *Client) ListAuditLogs(ctx context.Context, p ListAuditLogsParams) (AuditLogList, error) {
	q := url.Values{}
	setString(q, "before", p.Before)
	setInt(q, "limit", int64(p.Limit))
	return c.call[AuditLogList](ctx, request{method: http.MethodGet, path: []string{"audit-logs"}, query: q})
}

// ---- 補助 ----

func formatID(id int64) string { return strconv.FormatInt(id, 10) }

func setString(q url.Values, name, v string) {
	if v != "" {
		q.Set(name, v)
	}
}

func setInt(q url.Values, name string, v int64) {
	if v != 0 {
		q.Set(name, strconv.FormatInt(v, 10))
	}
}
