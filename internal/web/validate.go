package web

import (
	"cmp"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/oyaguma3/web-gui-for-aka-only-server/internal/adminapi"
)

// 管理API の入力形式。管理API に送る前に BFF でも確かめ、誤りを日本語で示す。
var (
	imsiPattern  = regexp.MustCompile(`^[0-9]{5,15}$`)
	hex128       = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)
	sqnPattern   = regexp.MustCompile(`^[0-9a-fA-F]{12}$`)
	amfPattern   = regexp.MustCompile(`^[0-9a-fA-F]{4}$`)
	prefixDigits = regexp.MustCompile(`^[0-9]{1,15}$`)
)

// sqnTypes は SQN 増加タイプの選択肢。
var sqnTypes = []struct{ Value, Label string }{
	{adminapi.SQNTypeInc32, "inc32（SEQ+1、IND 維持）"},
	{adminapi.SQNTypeInc33, "inc33（SEQ+1、IND+1）"},
	{adminapi.SQNTypeInc1, "inc1（SQN+1）"},
}

func validSQNType(v string) bool {
	for _, t := range sqnTypes {
		if t.Value == v {
			return true
		}
	}
	return false
}

// fieldErrors は項目ごとの入力の誤り。
type fieldErrors map[string]string

func (f fieldErrors) check(ok bool, field, msg string) {
	if !ok {
		if _, dup := f[field]; !dup {
			f[field] = msg
		}
	}
}

// normalizeHex は 16 進の入力から前後の空白を除き、小文字にする。
func normalizeHex(v string) string { return strings.ToLower(strings.TrimSpace(v)) }

// fieldLabels は管理API の項目名を画面の名前にする。
var fieldLabels = map[string]string{
	"imsi":             "IMSI",
	"ki":               "Ki",
	"opc":              "OPc",
	"sqn":              "SQN",
	"amf":              "AMF",
	"sqnType":          "SQN 増加タイプ",
	"allowPlain":       "平文HTTP",
	"allowedClientIds": "許可クライアント",
	"name":             "名前",
	"certPem":          "証明書",
	"keyPem":           "秘密鍵",
	"networkName":      "Network Name",
	"enabled":          "有効",
}

// apiErrorMessage は管理API の呼び出しのエラーを、ステータスと利用者向けの説明にする。
// notFound は対象が見つからないときの説明。
func apiErrorMessage(err error, notFound string) (int, string) {
	if adminapi.IsUnavailable(err) {
		return http.StatusBadGateway, adminErrorMessage(err)
	}
	apiErr, ok := errors.AsType[*adminapi.Error](err)
	if !ok {
		return http.StatusInternalServerError, "処理に失敗しました。"
	}
	switch apiErr.Problem.Cause {
	case adminapi.CauseUserNotFound:
		return http.StatusNotFound, cmp.Or(notFound, "加入者が見つかりません。")
	case adminapi.CauseSubscriberExists:
		return http.StatusConflict, "その IMSI の加入者は既に登録されています。"
	case adminapi.CauseClientNotFound:
		if apiErr.Status == http.StatusNotFound {
			return http.StatusNotFound, cmp.Or(notFound, "AVクライアントが見つかりません。")
		}
		return http.StatusBadRequest, "許可クライアントに、存在しない AVクライアントが含まれています。画面を開き直してください。"
	case adminapi.CauseCertRegistered:
		return http.StatusConflict, "その証明書は、既に別の AVクライアントに登録されています。"
	case adminapi.CauseInvalidCertificate:
		return http.StatusBadRequest, "証明書または秘密鍵を読み取れないか、証明書が有効期間外（期限切れ、または開始前）か、秘密鍵が証明書と対応していません。"
	}
	if apiErr.Status == http.StatusBadRequest && len(apiErr.Problem.InvalidParams) > 0 {
		var names []string
		for _, p := range apiErr.Problem.InvalidParams {
			names = append(names, cmp.Or(fieldLabels[p.Param], p.Param))
		}
		return http.StatusBadRequest, "入力が正しくありません（" + strings.Join(names, "、") + "）。"
	}
	if apiErr.Status == http.StatusNotFound {
		return http.StatusNotFound, cmp.Or(notFound, "対象が見つかりません。")
	}
	return http.StatusBadGateway, "aka-only-server の管理API がエラーを返しました。"
}
