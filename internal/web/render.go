package web

import (
	"bytes"
	"cmp"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/oyaguma3/web-gui-for-aka-only-server/internal/adminapi"
	"github.com/oyaguma3/web-gui-for-aka-only-server/internal/auth"
)

// pages はページ名（templates/pages/ のファイル名から拡張子を除いたもの）ごとのテンプレート。
// 各ページは templates/layout.html の "layout" を実行し、ページ側で "content" を定義する。
// ページごとに別々に解析するので、ページのファイルで定義したテンプレートは他のページからは使えない。
type pages map[string]*template.Template

// funcs はテンプレートで使う関数。
var funcs = template.FuncMap{
	// expiry は証明書の有効期限の状態を返す。期限切れと 30 日以内を目立たせる。問題がなければ nil
	// （テンプレートの with は、中身が空の構造体でも真として扱うので、ポインターで返す）。
	"expiry": func(notAfter time.Time) *expiryState {
		switch d := time.Until(notAfter); {
		case notAfter.IsZero():
			return nil
		case d <= 0:
			return &expiryState{Class: "expired", Label: "期限切れ"}
		case d <= 30*24*time.Hour:
			return &expiryState{Class: "expiring", Label: fmt.Sprintf("あと %d 日", int(d.Hours()/24))}
		}
		return nil
	},
	// short はフィンガープリントなどの長い値を先頭だけにする。
	"short": func(s string) string {
		if len(s) <= 16 {
			return s
		}
		return s[:16] + "…"
	},
	// attrs はログの属性を key=value の並びにする。
	"attrs": formatAttrs,
	// newestFirst はログを新しい順に並べ替える。
	"newestFirst": func(items []adminapi.LogEntry) []adminapi.LogEntry {
		out := slices.Clone(items)
		slices.Reverse(out)
		return out
	},
	// actionLabel は監査ログの操作の名前を返す。
	"actionLabel": func(action string) string { return cmp.Or(auditActionLabels[action], action) },
	// certInput は証明書や秘密鍵の入力欄（partials/cert.html）に渡す値を作る。
	"certInput": func(name, label, value, errMsg string) certInputData {
		return certInputData{Name: name, Label: label, Value: value, Error: errMsg}
	},
	// keysPlaceholder は、Ki / OPc をまだ表示していない状態の値を作る。
	"keysPlaceholder": func(imsi string) keysData { return keysData{IMSI: imsi} },
	// accessFields は、詳細の画面で許可クライアントと平文HTTP の入力欄に渡す値を作る。
	"accessFields": func(d subscriberData) subscriberForm {
		return subscriberForm{Clients: d.Clients, AllowPlain: d.Sub.AllowPlain}
	},
	// minPasswordLen はパスワードの最小文字数。入力欄の制限と説明に使う。
	"minPasswordLen": func() int { return auth.MinPasswordLen },
	// datetime は日時を BFF のタイムゾーン（環境変数 TZ）で表示する。
	"datetime": func(t time.Time) string {
		if t.IsZero() {
			return "-"
		}
		return t.Local().Format("2006-01-02 15:04:05 MST")
	},
}

func parsePages() (pages, error) {
	// 共通の部品（templates/partials/）は全てのページで使える。
	// missingkey=zero: 入力の誤り（fieldErrors）のように、マップにない項目を空文字列として扱う。
	base, err := template.New("layout.html").Funcs(funcs).Option("missingkey=zero").
		ParseFS(assets, "templates/layout.html", "templates/partials/*.html")
	if err != nil {
		return nil, err
	}
	files, err := fs.Glob(assets, "templates/pages/*.html")
	if err != nil {
		return nil, err
	}
	p := pages{}
	for _, file := range files {
		t, err := template.Must(base.Clone()).ParseFS(assets, file)
		if err != nil {
			return nil, err
		}
		p[strings.TrimSuffix(path.Base(file), ".html")] = t
	}
	return p, nil
}

// certInputData は証明書や秘密鍵の入力欄に渡す値。
type certInputData struct {
	Name, Label, Value, Error string
}

// expiryState は証明書の有効期限の状態。
type expiryState struct {
	Class string // expired / expiring
	Label string
}

// view はテンプレートに渡す値。
type view struct {
	Title   string
	Version string
	// User はログイン中のアカウント。ログインしていなければ nil。
	User *auth.Account
	// Data はページごとの値。
	Data any
}

// render はページをレイアウトに入れて描画して返す。
func (h *Handler) render(w http.ResponseWriter, r *http.Request, status int, page, title string, data any) {
	t, ok := h.pages[page]
	if !ok {
		h.log.Error("unknown page template", "page", page)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	v := view{Title: title, Version: h.version, Data: data}
	if a, ok := accountFrom(r.Context()); ok {
		v.User = &a
	}
	h.execute(w, r, status, t, "layout", v)
}

// renderBlock はページの中の 1 つのテンプレート（htmx で差し替える部分）だけを描画して返す。
func (h *Handler) renderBlock(w http.ResponseWriter, r *http.Request, status int, page, block string, data any) {
	t, ok := h.pages[page]
	if !ok {
		h.log.Error("unknown page template", "page", page)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	h.execute(w, r, status, t, block, data)
}

// execute はテンプレートを描画して返す。描画が終わってから書き出すので、途中で失敗しても壊れた HTML は返さない。
func (h *Handler) execute(w http.ResponseWriter, r *http.Request, status int, t *template.Template, name string, data any) {
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, name, data); err != nil {
		h.log.Error("render template", "template", name, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	hdr := w.Header()
	hdr.Set("Content-Type", "text/html; charset=utf-8")
	hdr.Set("Content-Length", strconv.Itoa(buf.Len()))
	// 画面には加入者情報や鍵情報が載るので、ブラウザにもキャッシュさせない。
	hdr.Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		w.Write(buf.Bytes())
	}
}

// errorData はエラーページに渡す値。
type errorData struct {
	Status  int
	Message string
	// Link と LinkLabel は案内するリンク。空ならホームへのリンクを出す。
	Link      string
	LinkLabel string
}

func (h *Handler) renderError(w http.ResponseWriter, r *http.Request, status int, message string) {
	data := errorData{Status: status, Message: message}
	if r.Header.Get("HX-Request") == "true" {
		// htmx の操作では、差し替え先に関わらず本文全体をエラーの表示に置き換える。
		w.Header().Set("HX-Retarget", "main")
		w.Header().Set("HX-Reswap", "innerHTML")
		h.renderBlock(w, r, status, "error", "content", view{Title: http.StatusText(status), Version: h.version, Data: data})
		return
	}
	h.render(w, r, status, "error", http.StatusText(status), data)
}
