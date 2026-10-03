package web

import (
	"net/http"
	"strings"

	"github.com/oyaguma3/web-gui-for-aka-only-server/internal/adminapi"
)

// serverCertData は AV用サーバー証明書の画面に渡す値。
type serverCertData struct {
	Cert    adminapi.ServerCertificate
	CanEdit bool
	Message string
	Error   string
	Errors  fieldErrors
	// CertPEM は登録フォームの証明書の入力（誤りのときに戻す）。秘密鍵は戻さない。
	CertPEM string
}

func (h *Handler) loadServerCert(r *http.Request) (serverCertData, int, string) {
	me, _ := accountFrom(r.Context())
	d := serverCertData{CanEdit: me.IsAdmin(), Errors: fieldErrors{}}
	c, err := h.admin.AVServerCertificate(r.Context())
	if err != nil {
		status, msg := apiErrorMessage(err, "")
		h.log.Warn("get av server certificate", "error", err)
		return d, status, msg
	}
	d.Cert = c
	return d, http.StatusOK, ""
}

func (h *Handler) serverCert(w http.ResponseWriter, r *http.Request) {
	d, status, msg := h.loadServerCert(r)
	d.Error = msg
	h.render(w, r, status, "servercert", "AV用サーバー証明書", d)
}

// serverCertPEM は AV用サーバー証明書を PEM ファイルとして返す。AVクライアントに配るときに使う。
func (h *Handler) serverCertPEM(w http.ResponseWriter, r *http.Request) {
	c, err := h.admin.AVServerCertificate(r.Context())
	if err != nil {
		status, msg := apiErrorMessage(err, "")
		h.log.Warn("get av server certificate", "error", err)
		h.renderError(w, r, status, msg)
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", `attachment; filename="aka-av-server.pem"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(c.CertPEM))
}

// renderServerCert は操作の結果を返す（htmx では証明書の部分だけ）。
func (h *Handler) renderServerCert(w http.ResponseWriter, r *http.Request, status int, mutate func(*serverCertData)) {
	d, loadStatus, msg := h.loadServerCert(r)
	if loadStatus != http.StatusOK {
		h.renderError(w, r, loadStatus, msg)
		return
	}
	mutate(&d)
	if r.Header.Get("HX-Request") == "true" {
		h.renderBlock(w, r, status, "servercert", "servercert-section", d)
		return
	}
	h.render(w, r, status, "servercert", "AV用サーバー証明書", d)
}

// serverCertReplace は持ち込みの証明書と秘密鍵に差し替える。管理者だけが使える。
func (h *Handler) serverCertReplace(w http.ResponseWriter, r *http.Request) {
	if err := parseForm(w, r); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "入力を読み取れませんでした（証明書と秘密鍵は合わせて 64KB までです）。")
		return
	}
	certPEM, err1 := pemFromForm(r, "cert_pem", "cert_file")
	keyPEM, err2 := pemFromForm(r, "key_pem", "key_file")
	errs := fieldErrors{}
	errs.check(err1 == nil && strings.Contains(certPEM, "-----BEGIN CERTIFICATE-----"), "cert", "PEM 形式の証明書を貼り付けるか、ファイルを選んでください。")
	errs.check(err2 == nil && strings.Contains(keyPEM, "PRIVATE KEY-----"), "key", "PEM 形式の秘密鍵（暗号化していないもの）を貼り付けるか、ファイルを選んでください。")
	if len(errs) > 0 {
		h.renderServerCert(w, r, http.StatusBadRequest, func(d *serverCertData) {
			d.Errors, d.Error, d.CertPEM = errs, "入力を確かめてください。", certPEM
		})
		return
	}
	if _, err := h.admin.ReplaceAVServerCertificate(r.Context(), certPEM, keyPEM); err != nil {
		status, msg := apiErrorMessage(err, "")
		h.log.Warn("replace av server certificate", "error", err)
		h.renderServerCert(w, r, status, func(d *serverCertData) { d.Error, d.CertPEM = msg, certPEM })
		return
	}
	h.renderServerCert(w, r, http.StatusOK, func(d *serverCertData) {
		d.Message = "AV用サーバー証明書を差し替えました。以後の新しい接続から使われます。AVクライアントの信頼設定を確かめてください。"
	})
}

// serverCertReset は登録済みの証明書を捨て、自己署名を再生成する。管理者だけが使える。
func (h *Handler) serverCertReset(w http.ResponseWriter, r *http.Request) {
	if _, err := h.admin.ResetAVServerCertificate(r.Context()); err != nil {
		status, msg := apiErrorMessage(err, "")
		h.log.Warn("reset av server certificate", "error", err)
		h.renderServerCert(w, r, status, func(d *serverCertData) { d.Error = msg })
		return
	}
	h.renderServerCert(w, r, http.StatusOK, func(d *serverCertData) {
		d.Message = "自己署名の証明書を作り直しました。AVクライアントには新しい証明書を配り直してください。"
	})
}
