package web

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/oyaguma3/web-gui-for-aka-only-server/internal/adminapi"
)

// subscribersPerPage は加入者の一覧の 1 ページの件数。
const subscribersPerPage = 50

// clientOption は許可クライアントの選択肢。
type clientOption struct {
	ID       int64
	Name     string
	Enabled  bool
	Selected bool
}

// clientOptions は AVクライアントの一覧から選択肢を作る。selected は選択済みの ID。
func clientOptions(clients []adminapi.AVClient, selected []int64) []clientOption {
	opts := make([]clientOption, len(clients))
	for i, c := range clients {
		opts[i] = clientOption{ID: c.ID, Name: c.Name, Enabled: c.Enabled, Selected: slices.Contains(selected, c.ID)}
	}
	return opts
}

// clientIDsFromForm はフォームの許可クライアント（clients=1&clients=3）を読む。
func clientIDsFromForm(form url.Values) ([]int64, bool) {
	ids := []int64{}
	for _, v := range form["clients"] {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id < 1 {
			return nil, false
		}
		ids = append(ids, id)
	}
	return ids, true
}

// ---- 一覧 ----

type subscriberListData struct {
	Prefix  string
	Items   []adminapi.Subscriber
	Total   int64
	Cursor  string // このページの cursor（先頭なら空）
	Next    string // 次のページの cursor
	Clients map[int64]string
	Message string
	Error   string
}

func (h *Handler) subscribers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	d := subscriberListData{Prefix: q.Get("prefix"), Cursor: q.Get("cursor")}
	if v := q.Get("deleted"); imsiPattern.MatchString(v) {
		d.Message = "加入者 " + v + " を削除しました。"
	}
	if d.Prefix != "" && !prefixDigits.MatchString(d.Prefix) {
		d.Error = "IMSI の前方一致は 15 桁までの数字で指定してください。"
		h.render(w, r, http.StatusBadRequest, "subscribers", "加入者", d)
		return
	}
	list, err := h.admin.ListSubscribers(r.Context(), adminapi.ListSubscribersParams{
		Prefix: d.Prefix, Cursor: d.Cursor, Limit: subscribersPerPage,
	})
	if err != nil {
		status, msg := apiErrorMessage(err, "")
		h.log.Warn("list subscribers", "error", err)
		d.Error = msg
		h.render(w, r, status, "subscribers", "加入者", d)
		return
	}
	d.Items, d.Total, d.Next = list.Items, list.Total, list.NextCursor
	d.Clients = h.clientNames(r.Context())
	h.render(w, r, http.StatusOK, "subscribers", "加入者", d)
}

// clientNames は AVクライアントの ID から名前を引く表を返す。取得できなければ空の表を返す。
func (h *Handler) clientNames(ctx context.Context) map[int64]string {
	names := map[int64]string{}
	clients, err := h.admin.ListAVClients(ctx)
	if err != nil {
		h.log.Warn("list av clients", "error", err)
		return names
	}
	for _, c := range clients {
		names[c.ID] = c.Name
	}
	return names
}

// ---- 登録 ----

type subscriberForm struct {
	IMSI, Ki, OPc, SQN, AMF, SQNType string
	AllowPlain                       bool
	Clients                          []clientOption
	SQNTypes                         []struct{ Value, Label string }
	Errors                           fieldErrors
	Error                            string
}

func (h *Handler) newSubscriberForm(ctx context.Context, f *subscriberForm, selected []int64) error {
	f.SQNTypes = sqnTypes
	clients, err := h.admin.ListAVClients(ctx)
	if err != nil {
		return err
	}
	f.Clients = clientOptions(clients, selected)
	return nil
}

func (h *Handler) subscriberNew(w http.ResponseWriter, r *http.Request) {
	f := subscriberForm{SQN: "000000000000", AMF: "8000", SQNType: adminapi.SQNTypeInc32}
	if err := h.newSubscriberForm(r.Context(), &f, nil); err != nil {
		status, msg := apiErrorMessage(err, "")
		h.log.Warn("list av clients", "error", err)
		f.Error = msg
		h.render(w, r, status, "subscriber_new", "加入者の登録", f)
		return
	}
	h.render(w, r, http.StatusOK, "subscriber_new", "加入者の登録", f)
}

func (h *Handler) subscriberCreate(w http.ResponseWriter, r *http.Request) {
	if err := parseForm(w, r); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "入力を読み取れませんでした。")
		return
	}
	pf := r.PostForm
	f := subscriberForm{
		IMSI: pf.Get("imsi"), Ki: normalizeHex(pf.Get("ki")), OPc: normalizeHex(pf.Get("opc")),
		SQN: normalizeHex(pf.Get("sqn")), AMF: normalizeHex(pf.Get("amf")), SQNType: pf.Get("sqn_type"),
		AllowPlain: pf.Get("allow_plain") == "on", Errors: fieldErrors{},
	}
	ids, idsOK := clientIDsFromForm(pf)
	f.Errors.check(imsiPattern.MatchString(f.IMSI), "imsi", "IMSI は 5〜15 桁の数字で入力してください。")
	f.Errors.check(hex128.MatchString(f.Ki), "ki", "Ki は 16 進 32 桁で入力してください。")
	f.Errors.check(hex128.MatchString(f.OPc), "opc", "OPc は 16 進 32 桁で入力してください。")
	f.Errors.check(sqnPattern.MatchString(f.SQN), "sqn", "SQN は 16 進 12 桁で入力してください。")
	f.Errors.check(amfPattern.MatchString(f.AMF), "amf", "AMF は 16 進 4 桁で入力してください。")
	f.Errors.check(validSQNType(f.SQNType), "sqn_type", "SQN 増加タイプを選んでください。")
	f.Errors.check(idsOK, "clients", "許可クライアントの指定が正しくありません。")

	fail := func(status int) {
		if err := h.newSubscriberForm(r.Context(), &f, ids); err != nil && f.Error == "" {
			_, f.Error = apiErrorMessage(err, "")
		}
		h.render(w, r, status, "subscriber_new", "加入者の登録", f)
	}
	if len(f.Errors) > 0 {
		fail(http.StatusBadRequest)
		return
	}
	_, err := h.admin.CreateSubscriber(r.Context(), adminapi.SubscriberCreate{
		IMSI: f.IMSI, Ki: f.Ki, OPc: f.OPc, SQN: f.SQN, AMF: f.AMF, SQNType: f.SQNType,
		AllowPlain: f.AllowPlain, AllowedClientIDs: ids,
	})
	if err != nil {
		status, msg := apiErrorMessage(err, "")
		h.log.Warn("create subscriber", "error", err)
		f.Error = msg
		fail(status)
		return
	}
	http.Redirect(w, r, "/subscribers/"+f.IMSI+"?created=1", http.StatusSeeOther)
}

// ---- 詳細と変更 ----

type subscriberData struct {
	Sub      adminapi.Subscriber
	Clients  []clientOption
	SQNTypes []struct{ Value, Label string }
	// CanEditAuth は Ki / OPc / SQN / AMF / SQN 増加タイプを変更・閲覧できるか（管理者のみ）。
	CanEditAuth bool
	Message     string
	Error       string
	Errors      fieldErrors
}

// loadSubscriber は詳細の画面に出す値を集める。取得できなければ、書き出す応答のステータスと説明を返す。
func (h *Handler) loadSubscriber(r *http.Request, imsi string) (subscriberData, int, string) {
	me, _ := accountFrom(r.Context())
	d := subscriberData{SQNTypes: sqnTypes, CanEditAuth: me.IsAdmin(), Errors: fieldErrors{}}
	sub, err := h.admin.GetSubscriber(r.Context(), imsi)
	if err != nil {
		status, msg := apiErrorMessage(err, "加入者 "+imsi+" は登録されていません。")
		if status != http.StatusNotFound {
			h.log.Warn("get subscriber", "error", err)
		}
		return d, status, msg
	}
	d.Sub = sub
	clients, err := h.admin.ListAVClients(r.Context())
	if err != nil {
		status, msg := apiErrorMessage(err, "")
		h.log.Warn("list av clients", "error", err)
		return d, status, msg
	}
	d.Clients = clientOptions(clients, sub.AllowedClientIDs)
	return d, http.StatusOK, ""
}

// subscriberIMSI は URL の IMSI を返す。形式が違えば 404 を返して false。
func (h *Handler) subscriberIMSI(w http.ResponseWriter, r *http.Request) (string, bool) {
	imsi := r.PathValue("imsi")
	if !imsiPattern.MatchString(imsi) {
		h.notFound(w, r)
		return "", false
	}
	return imsi, true
}

func (h *Handler) subscriber(w http.ResponseWriter, r *http.Request) {
	imsi, ok := h.subscriberIMSI(w, r)
	if !ok {
		return
	}
	d, status, msg := h.loadSubscriber(r, imsi)
	if status != http.StatusOK {
		h.renderError(w, r, status, msg)
		return
	}
	if r.URL.Query().Get("created") == "1" {
		d.Message = "加入者を登録しました。"
	}
	h.render(w, r, http.StatusOK, "subscriber", "加入者 "+imsi, d)
}

// renderSubscriber は変更の結果を詳細の画面（htmx では詳細の部分だけ）で返す。
func (h *Handler) renderSubscriber(w http.ResponseWriter, r *http.Request, imsi string, status int, mutate func(*subscriberData)) {
	d, loadStatus, msg := h.loadSubscriber(r, imsi)
	if loadStatus != http.StatusOK {
		h.renderError(w, r, loadStatus, msg)
		return
	}
	mutate(&d)
	if r.Header.Get("HX-Request") == "true" {
		h.renderBlock(w, r, status, "subscriber", "subscriber-detail", d)
		return
	}
	h.render(w, r, status, "subscriber", "加入者 "+imsi, d)
}

// subscriberAccess は許可クライアントと平文HTTP の許可を変更する。全員が使える。
func (h *Handler) subscriberAccess(w http.ResponseWriter, r *http.Request) {
	imsi, ok := h.subscriberIMSI(w, r)
	if !ok {
		return
	}
	if err := parseForm(w, r); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "入力を読み取れませんでした。")
		return
	}
	ids, idsOK := clientIDsFromForm(r.PostForm)
	if !idsOK {
		h.renderSubscriber(w, r, imsi, http.StatusBadRequest, func(d *subscriberData) {
			d.Error = "許可クライアントの指定が正しくありません。"
		})
		return
	}
	_, err := h.admin.UpdateSubscriber(r.Context(), imsi, adminapi.SubscriberUpdate{
		AllowPlain:       new(r.PostForm.Get("allow_plain") == "on"),
		AllowedClientIDs: &ids,
	})
	if err != nil {
		status, msg := apiErrorMessage(err, "")
		h.log.Warn("update subscriber access", "error", err)
		h.renderSubscriber(w, r, imsi, status, func(d *subscriberData) { d.Error = msg })
		return
	}
	h.renderSubscriber(w, r, imsi, http.StatusOK, func(d *subscriberData) {
		d.Message = "許可クライアントと平文HTTP の設定を変更しました。"
	})
}

// subscriberAuth は Ki / OPc / SQN / AMF / SQN 増加タイプを変更する。管理者だけが使える。
// 入力のあった項目と、値が変わった項目だけを送る。
func (h *Handler) subscriberAuth(w http.ResponseWriter, r *http.Request) {
	imsi, ok := h.subscriberIMSI(w, r)
	if !ok {
		return
	}
	if err := parseForm(w, r); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "入力を読み取れませんでした。")
		return
	}
	cur, err := h.admin.GetSubscriber(r.Context(), imsi)
	if err != nil {
		status, msg := apiErrorMessage(err, "加入者 "+imsi+" は登録されていません。")
		h.renderError(w, r, status, msg)
		return
	}
	pf := r.PostForm
	ki, opc := normalizeHex(pf.Get("ki")), normalizeHex(pf.Get("opc"))
	sqn, amf, sqnType := normalizeHex(pf.Get("sqn")), normalizeHex(pf.Get("amf")), pf.Get("sqn_type")
	errs := fieldErrors{}
	errs.check(ki == "" || hex128.MatchString(ki), "ki", "Ki は 16 進 32 桁で入力してください。")
	errs.check(opc == "" || hex128.MatchString(opc), "opc", "OPc は 16 進 32 桁で入力してください。")
	errs.check(sqnPattern.MatchString(sqn), "sqn", "SQN は 16 進 12 桁で入力してください。")
	errs.check(amfPattern.MatchString(amf), "amf", "AMF は 16 進 4 桁で入力してください。")
	errs.check(validSQNType(sqnType), "sqn_type", "SQN 増加タイプを選んでください。")
	if len(errs) > 0 {
		h.renderSubscriber(w, r, imsi, http.StatusBadRequest, func(d *subscriberData) {
			d.Errors = errs
			d.Error = "入力を確かめてください。"
		})
		return
	}

	var u adminapi.SubscriberUpdate
	var changed []string
	if ki != "" {
		u.Ki, changed = &ki, append(changed, "Ki")
	}
	if opc != "" {
		u.OPc, changed = &opc, append(changed, "OPc")
	}
	if sqn != cur.SQN {
		u.SQN, changed = &sqn, append(changed, "SQN")
	}
	if amf != cur.AMF {
		u.AMF, changed = &amf, append(changed, "AMF")
	}
	if sqnType != cur.SQNType {
		u.SQNType, changed = &sqnType, append(changed, "SQN 増加タイプ")
	}
	if len(changed) == 0 {
		h.renderSubscriber(w, r, imsi, http.StatusOK, func(d *subscriberData) { d.Message = "変更はありません。" })
		return
	}
	if _, err := h.admin.UpdateSubscriber(r.Context(), imsi, u); err != nil {
		status, msg := apiErrorMessage(err, "")
		h.log.Warn("update subscriber auth", "error", err)
		h.renderSubscriber(w, r, imsi, status, func(d *subscriberData) { d.Error = msg })
		return
	}
	h.renderSubscriber(w, r, imsi, http.StatusOK, func(d *subscriberData) {
		d.Message = strings.Join(changed, "、") + " を変更しました。"
	})
}

// subscriberKeys は Ki と OPc を表示する。管理者だけが使える。取得は aka-only-server の監査ログに残る。
func (h *Handler) subscriberKeys(w http.ResponseWriter, r *http.Request) {
	imsi, ok := h.subscriberIMSI(w, r)
	if !ok {
		return
	}
	keys, err := h.admin.GetSubscriberKeys(r.Context(), imsi)
	if err != nil {
		status, msg := apiErrorMessage(err, "加入者 "+imsi+" は登録されていません。")
		h.log.Warn("get subscriber keys", "error", err)
		h.renderBlock(w, r, status, "subscriber", "subscriber-keys", keysData{IMSI: imsi, Error: msg})
		return
	}
	h.renderBlock(w, r, http.StatusOK, "subscriber", "subscriber-keys", keysData{IMSI: imsi, Keys: &keys})
}

type keysData struct {
	IMSI  string
	Keys  *adminapi.SubscriberKeys
	Error string
}

// subscriberDelete は加入者を削除する。全員が使える。
func (h *Handler) subscriberDelete(w http.ResponseWriter, r *http.Request) {
	imsi, ok := h.subscriberIMSI(w, r)
	if !ok {
		return
	}
	if err := h.admin.DeleteSubscriber(r.Context(), imsi); err != nil {
		status, msg := apiErrorMessage(err, "加入者 "+imsi+" は登録されていません。")
		h.log.Warn("delete subscriber", "error", err)
		h.renderSubscriber(w, r, imsi, status, func(d *subscriberData) { d.Error = msg })
		return
	}
	seeOther(w, r, "/subscribers?deleted="+imsi)
}
