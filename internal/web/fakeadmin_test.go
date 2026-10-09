package web

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oyaguma3/web-gui-for-aka-only-server/internal/adminapi"
	"github.com/oyaguma3/web-gui-for-aka-only-server/internal/trace"
)

// fakeAdmin はテスト用の管理API。加入者と AVクライアントをメモリに持ち、呼び出しごとに操作者を記録する。
// err を設定すると、全ての呼び出しがそのエラーを返す。
type fakeAdmin struct {
	mu      sync.Mutex
	status  adminapi.Status
	err     error
	subs    map[string]adminapi.Subscriber
	keys    map[string]adminapi.SubscriberKeys
	clients map[int64]adminapi.AVClient
	nextID  int64
	cert    adminapi.ServerCertificate
	logs    []adminapi.LogEntry
	audit   []adminapi.AuditLogEntry
	// calls は「操作名 操作者」の記録。
	calls []string
	// traces は呼び出しごとのトレースID（コンテキストに入っていたもの）。
	traces []string
}

func newFakeAdmin() *fakeAdmin {
	return &fakeAdmin{
		status:  adminapi.Status{Version: "1.0.0"},
		subs:    map[string]adminapi.Subscriber{},
		keys:    map[string]adminapi.SubscriberKeys{},
		clients: map[int64]adminapi.AVClient{},
		nextID:  1,
		cert: adminapi.ServerCertificate{
			CertPEM: "-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n", Source: adminapi.CertSourceSelfSigned,
			FingerprintSHA256: strings.Repeat("ab", 32), Subject: "CN=aka-only-server",
			NotBefore: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), NotAfter: time.Date(2036, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}
}

func fakeWithStatus(st adminapi.Status) *fakeAdmin {
	f := newFakeAdmin()
	f.status = st
	return f
}

func fakeWithErr(err error) *fakeAdmin {
	f := newFakeAdmin()
	f.err = err
	return f
}

func notFound(cause string) error {
	return &adminapi.Error{Status: 404, Problem: adminapi.Problem{Cause: cause}}
}

// record は呼び出しを記録し、設定されたエラーを返す。
func (f *fakeAdmin) record(ctx context.Context, op string) error {
	f.calls = append(f.calls, op+" "+adminapi.OperatorFrom(ctx))
	f.traces = append(f.traces, trace.From(ctx))
	return f.err
}

func (f *fakeAdmin) lastCall() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return ""
	}
	return f.calls[len(f.calls)-1]
}

// lastCallOf は、操作名が op の最後の呼び出しを返す。
func (f *fakeAdmin) lastCallOf(op string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.calls) - 1; i >= 0; i-- {
		if strings.HasPrefix(f.calls[i], op+" ") {
			return f.calls[i]
		}
	}
	return ""
}

func (f *fakeAdmin) Status(ctx context.Context) (adminapi.Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record(ctx, "status"); err != nil {
		return adminapi.Status{}, err
	}
	st := f.status
	st.SubscriberCount, st.ClientCount = cmp.Or(st.SubscriberCount, int64(len(f.subs))), cmp.Or(st.ClientCount, int64(len(f.clients)))
	return st, nil
}

func (f *fakeAdmin) ListSubscribers(ctx context.Context, p adminapi.ListSubscribersParams) (adminapi.SubscriberList, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record(ctx, "list-subscribers"); err != nil {
		return adminapi.SubscriberList{}, err
	}
	var l adminapi.SubscriberList
	var rest []adminapi.Subscriber
	for _, imsi := range slices.Sorted(maps.Keys(f.subs)) {
		if strings.HasPrefix(imsi, p.Prefix) {
			l.Total++
			if imsi > p.Cursor {
				rest = append(rest, f.subs[imsi])
			}
		}
	}
	n := cmp.Or(p.Limit, 50)
	if len(rest) > n {
		rest = rest[:n]
		l.NextCursor = rest[n-1].IMSI
	}
	l.Items = rest
	return l, nil
}

func (f *fakeAdmin) CreateSubscriber(ctx context.Context, s adminapi.SubscriberCreate) (adminapi.Subscriber, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record(ctx, "create-subscriber"); err != nil {
		return adminapi.Subscriber{}, err
	}
	if _, ok := f.subs[s.IMSI]; ok {
		return adminapi.Subscriber{}, &adminapi.Error{Status: 409, Problem: adminapi.Problem{Cause: adminapi.CauseSubscriberExists}}
	}
	for _, id := range s.AllowedClientIDs {
		if _, ok := f.clients[id]; !ok {
			return adminapi.Subscriber{}, &adminapi.Error{Status: 400, Problem: adminapi.Problem{Cause: adminapi.CauseClientNotFound}}
		}
	}
	now := time.Now().UTC()
	sub := adminapi.Subscriber{
		IMSI: s.IMSI, SQN: cmp.Or(s.SQN, "000000000000"), AMF: cmp.Or(s.AMF, "8000"), SQNType: cmp.Or(s.SQNType, "inc32"),
		AllowPlain: s.AllowPlain, AllowedClientIDs: slices.Sorted(slices.Values(s.AllowedClientIDs)), CreatedAt: now, UpdatedAt: now,
	}
	f.subs[s.IMSI] = sub
	f.keys[s.IMSI] = adminapi.SubscriberKeys{Ki: strings.ToLower(s.Ki), OPc: strings.ToLower(s.OPc)}
	return sub, nil
}

func (f *fakeAdmin) GetSubscriber(ctx context.Context, imsi string) (adminapi.Subscriber, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record(ctx, "get-subscriber"); err != nil {
		return adminapi.Subscriber{}, err
	}
	sub, ok := f.subs[imsi]
	if !ok {
		return adminapi.Subscriber{}, notFound(adminapi.CauseUserNotFound)
	}
	return sub, nil
}

func (f *fakeAdmin) UpdateSubscriber(ctx context.Context, imsi string, u adminapi.SubscriberUpdate) (adminapi.Subscriber, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record(ctx, "update-subscriber"); err != nil {
		return adminapi.Subscriber{}, err
	}
	sub, ok := f.subs[imsi]
	if !ok {
		return adminapi.Subscriber{}, notFound(adminapi.CauseUserNotFound)
	}
	k := f.keys[imsi]
	if u.Ki != nil {
		k.Ki = *u.Ki
	}
	if u.OPc != nil {
		k.OPc = *u.OPc
	}
	if u.SQN != nil {
		sub.SQN = *u.SQN
	}
	if u.AMF != nil {
		sub.AMF = *u.AMF
	}
	if u.SQNType != nil {
		sub.SQNType = *u.SQNType
	}
	if u.AllowPlain != nil {
		sub.AllowPlain = *u.AllowPlain
	}
	if u.AllowedClientIDs != nil {
		sub.AllowedClientIDs = slices.Sorted(slices.Values(*u.AllowedClientIDs))
	}
	sub.UpdatedAt = time.Now().UTC()
	f.subs[imsi], f.keys[imsi] = sub, k
	return sub, nil
}

func (f *fakeAdmin) DeleteSubscriber(ctx context.Context, imsi string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record(ctx, "delete-subscriber"); err != nil {
		return err
	}
	if _, ok := f.subs[imsi]; !ok {
		return notFound(adminapi.CauseUserNotFound)
	}
	delete(f.subs, imsi)
	delete(f.keys, imsi)
	return nil
}

func (f *fakeAdmin) GetSubscriberKeys(ctx context.Context, imsi string) (adminapi.SubscriberKeys, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record(ctx, "get-keys"); err != nil {
		return adminapi.SubscriberKeys{}, err
	}
	k, ok := f.keys[imsi]
	if !ok {
		return adminapi.SubscriberKeys{}, notFound(adminapi.CauseUserNotFound)
	}
	return k, nil
}

func (f *fakeAdmin) ListAVClients(ctx context.Context) ([]adminapi.AVClient, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record(ctx, "list-clients"); err != nil {
		return nil, err
	}
	var out []adminapi.AVClient
	for _, id := range slices.Sorted(maps.Keys(f.clients)) {
		out = append(out, f.clients[id])
	}
	return out, nil
}

func (f *fakeAdmin) CreateAVClient(ctx context.Context, a adminapi.AVClientCreate) (adminapi.AVClient, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record(ctx, "create-client"); err != nil {
		return adminapi.AVClient{}, err
	}
	if !strings.Contains(a.CertPEM, "BEGIN CERTIFICATE") {
		return adminapi.AVClient{}, &adminapi.Error{Status: 400, Problem: adminapi.Problem{Cause: adminapi.CauseInvalidCertificate}}
	}
	for _, c := range f.clients {
		if strings.TrimSpace(c.CertPEM) == strings.TrimSpace(a.CertPEM) {
			return adminapi.AVClient{}, &adminapi.Error{Status: 409, Problem: adminapi.Problem{Cause: adminapi.CauseCertRegistered}}
		}
	}
	c := adminapi.AVClient{
		ID: f.nextID, Name: a.Name, CertPEM: a.CertPEM, FingerprintSHA256: strings.Repeat(strconv.FormatInt(f.nextID%10, 10), 64),
		Subject: "CN=" + a.Name, NotBefore: time.Now().UTC().Add(-time.Hour), NotAfter: time.Now().UTC().Add(365 * 24 * time.Hour),
		Enabled: a.Enabled, NetworkName: a.NetworkName, CreatedAt: time.Now().UTC(),
	}
	f.clients[c.ID] = c
	f.nextID++
	return c, nil
}

func (f *fakeAdmin) GetAVClient(ctx context.Context, id int64) (adminapi.AVClient, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record(ctx, "get-client"); err != nil {
		return adminapi.AVClient{}, err
	}
	c, ok := f.clients[id]
	if !ok {
		return adminapi.AVClient{}, notFound(adminapi.CauseClientNotFound)
	}
	return c, nil
}

func (f *fakeAdmin) UpdateAVClient(ctx context.Context, id int64, u adminapi.AVClientUpdate) (adminapi.AVClient, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record(ctx, "update-client"); err != nil {
		return adminapi.AVClient{}, err
	}
	c, ok := f.clients[id]
	if !ok {
		return adminapi.AVClient{}, notFound(adminapi.CauseClientNotFound)
	}
	if u.Name != nil {
		c.Name = *u.Name
	}
	if u.Enabled != nil {
		c.Enabled = *u.Enabled
	}
	if u.NetworkName != nil {
		c.NetworkName = *u.NetworkName
	}
	f.clients[id] = c
	return c, nil
}

func (f *fakeAdmin) DeleteAVClient(ctx context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record(ctx, "delete-client"); err != nil {
		return err
	}
	if _, ok := f.clients[id]; !ok {
		return notFound(adminapi.CauseClientNotFound)
	}
	delete(f.clients, id)
	for imsi, sub := range f.subs {
		sub.AllowedClientIDs = slices.DeleteFunc(sub.AllowedClientIDs, func(v int64) bool { return v == id })
		f.subs[imsi] = sub
	}
	return nil
}

func (f *fakeAdmin) ReplaceAVClientCertificate(ctx context.Context, id int64, certPEM string) (adminapi.AVClient, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record(ctx, "replace-client-cert"); err != nil {
		return adminapi.AVClient{}, err
	}
	c, ok := f.clients[id]
	if !ok {
		return adminapi.AVClient{}, notFound(adminapi.CauseClientNotFound)
	}
	if !strings.Contains(certPEM, "BEGIN CERTIFICATE") {
		return adminapi.AVClient{}, &adminapi.Error{Status: 400, Problem: adminapi.Problem{Cause: adminapi.CauseInvalidCertificate}}
	}
	c.CertPEM = certPEM
	c.FingerprintSHA256 = strings.Repeat("f", 64)
	f.clients[id] = c
	return c, nil
}

func (f *fakeAdmin) AVServerCertificate(ctx context.Context) (adminapi.ServerCertificate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record(ctx, "get-server-cert"); err != nil {
		return adminapi.ServerCertificate{}, err
	}
	return f.cert, nil
}

func (f *fakeAdmin) ReplaceAVServerCertificate(ctx context.Context, certPEM, keyPEM string) (adminapi.ServerCertificate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record(ctx, "replace-server-cert"); err != nil {
		return adminapi.ServerCertificate{}, err
	}
	if !strings.Contains(certPEM, "BEGIN CERTIFICATE") || !strings.Contains(keyPEM, "PRIVATE KEY") {
		return adminapi.ServerCertificate{}, &adminapi.Error{Status: 400, Problem: adminapi.Problem{Cause: adminapi.CauseInvalidCertificate}}
	}
	f.cert.CertPEM, f.cert.Source, f.cert.Subject = certPEM, adminapi.CertSourceUploaded, "CN=uploaded"
	return f.cert, nil
}

func (f *fakeAdmin) ResetAVServerCertificate(ctx context.Context) (adminapi.ServerCertificate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record(ctx, "reset-server-cert"); err != nil {
		return adminapi.ServerCertificate{}, err
	}
	f.cert.Source, f.cert.Subject = adminapi.CertSourceSelfSigned, "CN=aka-only-server"
	return f.cert, nil
}

func (f *fakeAdmin) ListLogs(ctx context.Context, p adminapi.ListLogsParams) (adminapi.LogList, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record(ctx, "list-logs"); err != nil {
		return adminapi.LogList{}, err
	}
	l := adminapi.LogList{BootID: "boot-1"}
	for _, e := range f.logs {
		if p.After == nil || e.Seq > *p.After {
			l.Items = append(l.Items, e)
		}
	}
	if n := cmp.Or(p.Limit, 200); p.After == nil && len(l.Items) > n {
		l.Items = l.Items[len(l.Items)-n:]
	}
	if len(l.Items) > 0 {
		l.LastSeq = l.Items[len(l.Items)-1].Seq
	} else if p.After != nil {
		l.LastSeq = *p.After
	}
	return l, nil
}

func (f *fakeAdmin) ListAuditLogs(ctx context.Context, p adminapi.ListAuditLogsParams) (adminapi.AuditLogList, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record(ctx, "list-audit"); err != nil {
		return adminapi.AuditLogList{}, err
	}
	var l adminapi.AuditLogList
	n := cmp.Or(p.Limit, 100)
	for i := len(f.audit) - 1; i >= 0; i-- {
		e := f.audit[i]
		if p.Before != "" && e.ID >= p.Before {
			continue
		}
		if len(l.Items) == n {
			l.NextBefore = l.Items[n-1].ID
			break
		}
		l.Items = append(l.Items, e)
	}
	return l, nil
}
