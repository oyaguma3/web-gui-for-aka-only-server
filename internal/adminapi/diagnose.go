package adminapi

import (
	"crypto/x509"
	"errors"
	"net"
	"strings"
)

// Diagnose は、管理API に接続できなかったときの原因の見当を、利用者向けの文で返す。
// 見当がつかなければ空文字列を返す。
func Diagnose(err error) string {
	if err == nil {
		return ""
	}
	if _, ok := errors.AsType[x509.HostnameError](err); ok {
		return "管理API のサーバー証明書に、接続先のホスト名（または IP アドレス）が入っていません。" +
			"aka-only-server 側の AKA_ADMIN_TLS_HOSTS に接続先を加えて admin-cert reset で作り直すか、" +
			"証明書に入っている名前で接続してください。"
	}
	if _, ok := errors.AsType[x509.UnknownAuthorityError](err); ok {
		return "管理API のサーバー証明書が、設定した証明書（WEBGUI_ADMIN_SERVER_CERT）と一致しません。" +
			"aka-only-server の admin-cert で取り出した証明書を設定してください。"
	}
	if _, ok := errors.AsType[x509.CertificateInvalidError](err); ok {
		return "管理API のサーバー証明書が有効期間外です。aka-only-server 側で admin-cert reset で作り直してください。"
	}
	// 相手から TLS のアラートを受け取ると、Op が "remote error" の *net.OpError になる。
	// aka-only-server は未登録のクライアント証明書を bad_certificate で拒否する。
	if opErr, ok := errors.AsType[*net.OpError](err); ok && opErr.Op == "remote error" &&
		(strings.Contains(opErr.Err.Error(), "bad certificate") || strings.Contains(opErr.Err.Error(), "certificate required")) {
		return "aka-only-server が BFF のクライアント証明書を受け付けませんでした。" +
			"aka-only-server 側の AKA_ADMIN_CLIENTS に、このクライアント証明書のフィンガープリントが登録されているか確認してください。"
	}
	if dnsErr, ok := errors.AsType[*net.DNSError](err); ok && dnsErr.IsNotFound {
		// 同一ホストの構成では、aka-only-server のコンテナが止まっているときも名前を解決できなくなる。
		return "管理API のホスト名（" + dnsErr.Name + "）を解決できません。aka-only-server が起動しているか、" +
			"同一ホストの場合は BFF が共有ネットワーク（aka-av）に参加しているか確認してください。"
	}
	if opErr, ok := errors.AsType[*net.OpError](err); ok && opErr.Op == "dial" {
		return "管理API に接続できません。aka-only-server が起動しているか、接続先の URL とポートが正しいか確認してください。"
	}
	if netErr, ok := errors.AsType[net.Error](err); ok && netErr.Timeout() {
		return "管理API から時間内に応答がありません。接続先のアドレス、VPN、aka-only-server 側の公開先（AKA_ADMIN_PUBLISH）を確認してください。"
	}
	return ""
}
