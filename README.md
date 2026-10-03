# web-gui-for-aka-only-server

[aka-only-server](https://github.com/oyaguma3/aka-only-server) の管理 GUI（BFF + Web GUI）です。aka-only-server の管理API だけを使って、加入者、AVクライアント、証明書を管理し、ログと監査ログを確認します。加入者のデータや鍵情報は自分では保存しません。

```
[ブラウザ] ──HTTPS（VPN 越し）──> BFF + Web GUI ──mTLS──> aka-only-server 管理API
                                      │
                                Valkey（BFF 専用）
```

個人利用・検証用途の GUI です。インターネットに直接公開せず、VPN（Tailscale、WireGuard など）越しに使うことを前提にしています。

## 特徴

- アカウントは 3 種類（最初の管理者、管理者、一般ユーザー）。権限は BFF で判定し、操作者を aka-only-server の監査ログに残します。
- Ki / OPc は管理者だけが表示・変更できます。一般ユーザーが扱えるのは、加入者の登録フォームの入力中だけです。
- パスワードは argon2id で保存し、ログイン試行の回数を制限します。セッションは無操作のタイムアウトとログインからの最大時間で切れます。
- ブラウザ向けの HTTPS は BFF 自身で終端します。証明書は自己署名（自動生成）か持ち込み（`tailscale cert` など。更新は再起動なしで反映）です。
- 画面は Go の `html/template` と htmx、Pico CSS で作っています。JavaScript は書いていません。

| 画面 | 内容 |
|---|---|
| ダッシュボード | 加入者数、AVクライアント数、証明書の期限、直近のログ |
| 加入者 | 一覧（IMSI の前方一致）、登録、詳細、変更、削除、Ki / OPc の表示（管理者） |
| AVクライアント | 一覧、詳細。登録、有効・無効、証明書の差し替え、削除（管理者） |
| AV用サーバー証明書 | 表示とダウンロード。持ち込みへの差し替え、自己署名の作り直し（管理者） |
| ログ | aka-only-server のサーバーログ（自動で追記） |
| 監査ログ | aka-only-server と BFF の監査ログ（管理者） |
| アカウント | アカウントの作成・削除・パスワード再設定（管理者） |

## 導入

Docker Engine と compose プラグインが必要です。aka-only-server を先に起動し、BFF を管理クライアントとして登録します。手順は[運用ガイド](docs/operation-guide.md)の 2 章にあります。

流れは次の通りです。

1. aka-only-server でクライアント証明書を作り、フィンガープリントを `AKA_ADMIN_CLIENTS` に登録する。管理API のサーバー証明書を取り出す。
2. 2 つの証明書を `certs/` に置く（所有者は UID 65532）。
3. `.env.example` を `.env` にコピーし、Valkey のパスワード、最初の管理者、公開するアドレスを書く。
4. 起動して、管理API に接続できることを確かめる。

```bash
docker compose up -d --build
```

```bash
docker compose exec aka-webgui /aka-webgui check-admin
```

5. ブラウザで開き、最初の管理者でログインして、日常の操作に使うアカウントを作る。

## ドキュメント

| ファイル | 内容 |
|---|---|
| [docs/operation-guide.md](docs/operation-guide.md) | 導入、ブラウザ向け HTTPS（自己署名 / Tailscale）、公開範囲、アカウントの運用、バックアップ、障害時の確認、環境変数 |
| [docs/design-overview.md](docs/design-overview.md) | 設計概要（アカウントと権限、セッション、管理API との連携、データモデル） |
| [docs/screen-spec.md](docs/screen-spec.md) | 画面仕様と権限ごとの表示差 |

## 開発

Go 1.27 以上が必要です。外部依存は `github.com/valkey-io/valkey-go` と `golang.org/x/crypto` だけです。

```bash
go test ./...
```

Valkey を使う結合テストと、aka-only-server の管理API を相手にした契約テストは、接続先を環境変数で指定したときだけ動きます。Valkey の結合テストは論理データベース 1 番の全データを消すので、専用の Valkey を使ってください。

```bash
WEBGUI_TEST_VALKEY_ADDR=127.0.0.1:16380 WEBGUI_TEST_VALKEY_PASSWORD=<パスワード> go test ./internal/store/
```

```bash
WEBGUI_TEST_ADMIN_URL=https://<host>:9443/admin/v1 WEBGUI_TEST_ADMIN_CLIENT_CERT=<bff.pem> WEBGUI_TEST_ADMIN_SERVER_CERT=<admin.pem> go test -run Integration ./internal/adminapi/
```

契約テストは、テスト用の加入者（IMSI が `00101` で始まるもの）と AVクライアントを作り、終わったら削除します。`WEBGUI_TEST_ADMIN_AV_CERT=1` を足すと、AV用サーバー証明書の差し替えと作り直しも試します（稼働中の AVクライアントに影響します）。

同梱しているサードパーティのファイル:

| ファイル | 版 | ライセンス |
|---|---|---|
| `internal/web/static/htmx.min.js` | htmx 2.0.11 | 0BSD |
| `internal/web/static/pico.min.css` | Pico CSS 2.1.1 | MIT |

## ライセンス

[LICENSE](LICENSE) を参照してください。
