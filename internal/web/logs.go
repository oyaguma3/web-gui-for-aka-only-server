package web

import (
	"encoding/json/v2"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/oyaguma3/web-gui-for-aka-only-server/internal/adminapi"
)

// logsLimit は、ログの画面を開いたときに読み込む件数。
const logsLimit = 200

// logLevels はログの絞り込みの選択肢。値以上のレベルを表示する。
var logLevels = []struct{ Value, Label string }{
	{"", "すべて"},
	{"INFO", "INFO 以上"},
	{"WARN", "WARN 以上"},
	{"ERROR", "ERROR のみ"},
}

var levelRank = map[string]int{"DEBUG": 0, "INFO": 1, "WARN": 2, "ERROR": 3}

// filterLogs は min 以上のレベルのログだけを返す。
func filterLogs(items []adminapi.LogEntry, min string) []adminapi.LogEntry {
	if min == "" {
		return items
	}
	return slices.DeleteFunc(slices.Clone(items), func(e adminapi.LogEntry) bool { return levelRank[e.Level] < levelRank[min] })
}

type logsData struct {
	Items   []adminapi.LogEntry
	BootID  string
	LastSeq int64
	Level   string
	Levels  []struct{ Value, Label string }
	Error   string
}

func validLevel(v string) string {
	if _, ok := levelRank[v]; ok {
		return v
	}
	return ""
}

// logs は aka-only-server のサーバーログの画面。全員が使える。
func (h *Handler) logs(w http.ResponseWriter, r *http.Request) {
	d := logsData{Level: validLevel(r.URL.Query().Get("level")), Levels: logLevels}
	l, err := h.admin.ListLogs(r.Context(), adminapi.ListLogsParams{Limit: logsLimit})
	if err != nil {
		status, msg := apiErrorMessage(err, "")
		h.log.Warn("list logs", "error", err)
		d.Error = msg
		h.render(w, r, status, "logs", "ログ", d)
		return
	}
	d.Items, d.BootID, d.LastSeq = filterLogs(l.Items, d.Level), l.BootID, l.LastSeq
	h.render(w, r, http.StatusOK, "logs", "ログ", d)
}

// logsTail は、ログの画面から定期的に呼ばれ、続きのログを返す。セッションの有効期限は延ばさない。
// aka-only-server が再起動していたら（bootId が変わっていたら）、画面を読み込み直させる。
func (h *Handler) logsTail(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	after, err := strconv.ParseInt(q.Get("after"), 10, 64)
	if err != nil || after < 0 {
		h.renderError(w, r, http.StatusBadRequest, "ログの位置の指定が正しくありません。")
		return
	}
	d := logsData{Level: validLevel(q.Get("level")), BootID: q.Get("boot"), LastSeq: after}
	l, err := h.admin.ListLogs(r.Context(), adminapi.ListLogsParams{After: &after, Limit: 1000})
	if err != nil {
		// 一時的に取得できなくても画面はそのままにし、次の呼び出しで続きを取る。
		h.log.Warn("tail logs", "error", err)
		_, d.Error = apiErrorMessage(err, "")
		h.renderBlock(w, r, http.StatusOK, "logs", "logs-tail", d)
		return
	}
	if l.BootID != d.BootID {
		w.Header().Set("HX-Refresh", "true")
		w.WriteHeader(http.StatusOK)
		return
	}
	d.Items, d.LastSeq = filterLogs(l.Items, d.Level), l.LastSeq
	h.renderBlock(w, r, http.StatusOK, "logs", "logs-tail", d)
}

// formatAttrs はログの属性を key=value の並びにする（キーの順）。
func formatAttrs(attrs map[string]any) string {
	var b strings.Builder
	for i, k := range slices.Sorted(maps.Keys(attrs)) {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(k)
		b.WriteByte('=')
		switch v := attrs[k].(type) {
		case string:
			b.WriteString(v)
		default:
			if j, err := json.Marshal(v); err == nil {
				b.Write(j)
			} else {
				fmt.Fprint(&b, v)
			}
		}
	}
	return b.String()
}
