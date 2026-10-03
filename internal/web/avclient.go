package web

import (
	"cmp"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/oyaguma3/web-gui-for-aka-only-server/internal/adminapi"
)

// AVクライアントの入力の上限（管理API と同じ）。
const (
	maxClientNameLen  = 64
	maxNetworkNameLen = 255
)

// ---- 一覧と登録 ----

type avClientsData struct {
	Clients []adminapi.AVClient
	CanEdit bool
	Message string
	Error   string
	Form    avClientForm
}

type avClientForm struct {
	Name, CertPEM, NetworkName string
	Enabled                    bool
	Errors                     fieldErrors
}

func (h *Handler) avClientsData(r *http.Request) (avClientsData, int, string) {
	me, _ := accountFrom(r.Context())
	d := avClientsData{CanEdit: me.IsAdmin(), Form: avClientForm{Enabled: true, Errors: fieldErrors{}}}
	clients, err := h.admin.ListAVClients(r.Context())
	if err != nil {
		status, msg := apiErrorMessage(err, "")
		h.log.Warn("list av clients", "error", err)
		return d, status, msg
	}
	d.Clients = clients
	return d, http.StatusOK, ""
}

func (h *Handler) avClients(w http.ResponseWriter, r *http.Request) {
	d, status, msg := h.avClientsData(r)
	d.Error = msg
	if v := r.URL.Query().Get("deleted"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			d.Message = "AVクライアント #" + strconv.FormatInt(id, 10) + " を削除しました。"
		}
	}
	h.render(w, r, status, "avclients", "AVクライアント", d)
}

// avClientCreate は AVクライアントを登録する。管理者だけが使える。
func (h *Handler) avClientCreate(w http.ResponseWriter, r *http.Request) {
	if err := parseForm(w, r); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "入力を読み取れませんでした（証明書は 64KB までです）。")
		return
	}
	certPEM, err := pemFromForm(r, "cert_pem", "cert_file")
	if err != nil {
		h.renderError(w, r, http.StatusBadRequest, "証明書のファイルを読み取れませんでした。")
		return
	}
	f := avClientForm{
		Name:        strings.TrimSpace(r.PostFormValue("name")),
		CertPEM:     certPEM,
		NetworkName: strings.TrimSpace(r.PostFormValue("network_name")),
		Enabled:     r.PostFormValue("enabled") == "on",
		Errors:      fieldErrors{},
	}
	f.Errors.check(f.Name != "" && len(f.Name) <= maxClientNameLen && utf8.ValidString(f.Name), "name", "名前は 1〜64 バイトで入力してください。")
	f.Errors.check(strings.Contains(f.CertPEM, "-----BEGIN CERTIFICATE-----"), "cert", "PEM 形式の証明書を貼り付けるか、ファイルを選んでください。")
	f.Errors.check(len(f.NetworkName) <= maxNetworkNameLen, "network_name", "Network Name は 255 バイトまでです。")

	fail := func(status int, msg string) {
		d, _, loadMsg := h.avClientsData(r)
		d.Form, d.Error = f, cmp.Or(msg, loadMsg)
		h.renderAVClients(w, r, status, d)
	}
	if len(f.Errors) > 0 {
		fail(http.StatusBadRequest, "入力を確かめてください。")
		return
	}
	c, err := h.admin.CreateAVClient(r.Context(), adminapi.AVClientCreate{
		Name: f.Name, CertPEM: f.CertPEM, Enabled: f.Enabled, NetworkName: f.NetworkName,
	})
	if err != nil {
		status, msg := apiErrorMessage(err, "")
		h.log.Warn("create av client", "error", err)
		fail(status, msg)
		return
	}
	d, status, msg := h.avClientsData(r)
	d.Error = msg
	d.Message = "AVクライアント " + c.Name + "（#" + strconv.FormatInt(c.ID, 10) + "）を登録しました。"
	h.renderAVClients(w, r, status, d)
}

// renderAVClients は一覧の画面（htmx では一覧と登録フォームの部分だけ）を返す。
func (h *Handler) renderAVClients(w http.ResponseWriter, r *http.Request, status int, d avClientsData) {
	if r.Header.Get("HX-Request") == "true" {
		h.renderBlock(w, r, status, "avclients", "avclients-section", d)
		return
	}
	h.render(w, r, status, "avclients", "AVクライアント", d)
}

// ---- 詳細と変更 ----

type avClientData struct {
	Client  adminapi.AVClient
	CanEdit bool
	Message string
	Error   string
	Errors  fieldErrors
	// Form は名前と Network Name の入力（誤りのときに戻す）。
	Form struct{ Name, NetworkName string }
}

// avClientID は URL のクライアントID を返す。形式が違えば 404 を返して false。
func (h *Handler) avClientID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		h.notFound(w, r)
		return 0, false
	}
	return id, true
}

func (h *Handler) loadAVClient(r *http.Request, id int64) (avClientData, int, string) {
	me, _ := accountFrom(r.Context())
	d := avClientData{CanEdit: me.IsAdmin(), Errors: fieldErrors{}}
	c, err := h.admin.GetAVClient(r.Context(), id)
	if err != nil {
		status, msg := apiErrorMessage(err, "AVクライアント #"+strconv.FormatInt(id, 10)+" は登録されていません。")
		if status != http.StatusNotFound {
			h.log.Warn("get av client", "error", err)
		}
		return d, status, msg
	}
	d.Client = c
	d.Form.Name, d.Form.NetworkName = c.Name, c.NetworkName
	return d, http.StatusOK, ""
}

func (h *Handler) avClient(w http.ResponseWriter, r *http.Request) {
	id, ok := h.avClientID(w, r)
	if !ok {
		return
	}
	d, status, msg := h.loadAVClient(r, id)
	if status != http.StatusOK {
		h.renderError(w, r, status, msg)
		return
	}
	h.render(w, r, http.StatusOK, "avclient", "AVクライアント "+d.Client.Name, d)
}

// renderAVClient は変更の結果を詳細の画面（htmx では詳細の部分だけ）で返す。
func (h *Handler) renderAVClient(w http.ResponseWriter, r *http.Request, id int64, status int, mutate func(*avClientData)) {
	d, loadStatus, msg := h.loadAVClient(r, id)
	if loadStatus != http.StatusOK {
		h.renderError(w, r, loadStatus, msg)
		return
	}
	mutate(&d)
	if r.Header.Get("HX-Request") == "true" {
		h.renderBlock(w, r, status, "avclient", "avclient-detail", d)
		return
	}
	h.render(w, r, status, "avclient", "AVクライアント "+d.Client.Name, d)
}

// avClientUpdate は名前、有効・無効、Network Name を変更する。管理者だけが使える。
func (h *Handler) avClientUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := h.avClientID(w, r)
	if !ok {
		return
	}
	if err := parseForm(w, r); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "入力を読み取れませんでした。")
		return
	}
	name := strings.TrimSpace(r.PostFormValue("name"))
	networkName := strings.TrimSpace(r.PostFormValue("network_name"))
	errs := fieldErrors{}
	errs.check(name != "" && len(name) <= maxClientNameLen && utf8.ValidString(name), "name", "名前は 1〜64 バイトで入力してください。")
	errs.check(len(networkName) <= maxNetworkNameLen, "network_name", "Network Name は 255 バイトまでです。")
	if len(errs) > 0 {
		h.renderAVClient(w, r, id, http.StatusBadRequest, func(d *avClientData) {
			d.Errors, d.Error = errs, "入力を確かめてください。"
			d.Form.Name, d.Form.NetworkName = name, networkName
		})
		return
	}
	_, err := h.admin.UpdateAVClient(r.Context(), id, adminapi.AVClientUpdate{
		Name: &name, Enabled: new(r.PostFormValue("enabled") == "on"), NetworkName: &networkName,
	})
	if err != nil {
		status, msg := apiErrorMessage(err, "AVクライアント #"+strconv.FormatInt(id, 10)+" は登録されていません。")
		h.log.Warn("update av client", "error", err)
		h.renderAVClient(w, r, id, status, func(d *avClientData) { d.Error = msg })
		return
	}
	h.renderAVClient(w, r, id, http.StatusOK, func(d *avClientData) { d.Message = "設定を変更しました。すぐに反映されます。" })
}

// avClientCertificate は証明書を差し替える。管理者だけが使える。
func (h *Handler) avClientCertificate(w http.ResponseWriter, r *http.Request) {
	id, ok := h.avClientID(w, r)
	if !ok {
		return
	}
	if err := parseForm(w, r); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "入力を読み取れませんでした（証明書は 64KB までです）。")
		return
	}
	certPEM, err := pemFromForm(r, "cert_pem", "cert_file")
	if err != nil || !strings.Contains(certPEM, "-----BEGIN CERTIFICATE-----") {
		h.renderAVClient(w, r, id, http.StatusBadRequest, func(d *avClientData) {
			d.Errors["cert"] = "PEM 形式の証明書を貼り付けるか、ファイルを選んでください。"
			d.Error = "入力を確かめてください。"
		})
		return
	}
	if _, err := h.admin.ReplaceAVClientCertificate(r.Context(), id, certPEM); err != nil {
		status, msg := apiErrorMessage(err, "AVクライアント #"+strconv.FormatInt(id, 10)+" は登録されていません。")
		h.log.Warn("replace av client certificate", "error", err)
		h.renderAVClient(w, r, id, status, func(d *avClientData) { d.Error = msg })
		return
	}
	h.renderAVClient(w, r, id, http.StatusOK, func(d *avClientData) {
		d.Message = "証明書を差し替えました。古い証明書では接続できなくなりました。"
	})
}

// avClientDelete は AVクライアントを削除する。管理者だけが使える。
func (h *Handler) avClientDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := h.avClientID(w, r)
	if !ok {
		return
	}
	if err := h.admin.DeleteAVClient(r.Context(), id); err != nil {
		status, msg := apiErrorMessage(err, "AVクライアント #"+strconv.FormatInt(id, 10)+" は登録されていません。")
		h.log.Warn("delete av client", "error", err)
		h.renderAVClient(w, r, id, status, func(d *avClientData) { d.Error = msg })
		return
	}
	seeOther(w, r, "/av-clients?deleted="+strconv.FormatInt(id, 10))
}
