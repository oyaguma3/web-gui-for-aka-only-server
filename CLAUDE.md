# web-gui-for-aka-only-server

aka-only-server の管理 GUI（BFF + Web GUI）。aka-only-server の管理API だけを使い、加入者データや鍵情報は自分では保存しない。

このファイルは、aka-only-server 側の検討・実装セッション（2026-10-03）からの引き継ぎメモを兼ねる。

## 現在の状態

- フェーズ3（BFF / Web GUI の実装）はこれから着手する。
- 旧実装（Gin + HTMX + Alpine.js）は流用しない方針で、コード・README・ユーザーガイドごと削除済み。README は新しい実装に合わせて書き直す。
- 作業はステップ単位で区切り、各ステップの終わりに「作ったもの」と「実際に動かして確かめたこと」を報告して確認をもらう。
  1. 骨格（環境変数による設定、slog、HTTPS 終端と自己署名の自動生成、テンプレートと静的ファイルの配信）… 実装済み
  2. 管理API クライアント（mTLS、`X-Operator-Id`、エラー処理）
  3. アカウントとセッション（argon2id、ログイン試行の制限、BFF 監査ログ、権限チェック）
  4. 画面（加入者 → AVクライアント → AV用サーバー証明書 → ログと監査ログ → アカウント管理 → ダッシュボード）
  5. compose、aka-only-server との通しの確認、`docs/operation-guide.md`、`README.md`
- `docs/screen-spec.md` は実装と並行して書く。最初の画面（加入者）ができた時点で書き起こし、画面ごとに追記する。
- aka-only-server はフェーズ1（認証ベクターAPI）とフェーズ2（管理API）を実装済み。場所は `/home/sumitakekino/projects/aka-only-server`。

## 最初に読むもの

| ファイル | 内容 |
|---|---|
| `docs/design-overview.md` | このリポジトリの設計。アカウントと権限、画面、データモデル |
| `../aka-only-server/docs/openapi/admin-api.yaml` | 管理API の仕様。BFF とサーバーの間の契約 |
| `../aka-only-server/docs/operation-guide.md` | 管理クライアントの登録、共有ネットワーク、証明書の扱い |
| `../aka-only-server/docs/design-overview.md` | システム全体の設計 |

## 実装方針（aka-only-server 側と同じ流儀にそろえる）

- Go 1.27.1。Go のコードを書く前に `modern-go-guidelines:use-modern-go` スキルでガイドラインを確認する。
- 標準ライブラリを優先する。外部依存は次の 2 つに限る。
  - `github.com/valkey-io/valkey-go`: BFF 専用 Valkey への接続
  - `golang.org/x/crypto`: argon2id（パスワードハッシュ）
- HTTP は `net/http`（メソッドつきの `ServeMux` パターンと `r.PathValue`）。Gin は使わない。
- JSON は `encoding/json/v2`。
- テンプレートは `html/template`、静的ファイルは `embed`。HTMX と Pico CSS はファイルを同梱して配信する。Alpine.js は使わない。
- ログは `log/slog` の JSON を標準出力に出す。ローテーションは Docker に任せる。
- 設定は環境変数だけで受け取る（`godotenv` は使わない）。compose の `.env` から渡す。
- module パスは `github.com/oyaguma3/web-gui-for-aka-only-server`。
- コードのコメントとドキュメントは日本語で書く。

## 設計上の決定事項（詳細は `docs/design-overview.md`）

- アカウント情報、権限、セッションは BFF が持つ。保存先は BFF 専用の Valkey。
- 最初の管理者は `.env` を常に正とし、Valkey には保存しない。パスワードは `.env` に平文で書く（パーミッション 600）。
- 権限判定は BFF だけで行う。aka-only-server は BFF を全権の管理クライアントとして扱う。
- 管理API を呼ぶときは、操作者のユーザーID を `X-Operator-Id` ヘッダーで渡す（形式は `^[A-Za-z0-9._@-]{1,64}$`）。ユーザーID の形式はこれに収まるように決める。
- Ki / OPc は `GET /subscribers/{imsi}/keys` でだけ取得できる。BFF は管理者にだけこれを呼ぶ。一般ユーザーが Ki / OPc を扱えるのは登録フォームの入力中だけ。
- 一般ユーザーは加入者を削除できる。削除して再登録すれば SQN / AMF を実質的に変更できるが、これは許容済み（サーバー側の監査ログで追跡できる）。
- 監査ログの閲覧は管理者のみ。通常のサーバーログは一般ユーザーも閲覧できる。
- ブラウザ向けの HTTPS は BFF 自身（Go）で終端する。リバースプロキシは置かない。証明書は自己署名（初回起動時に自動生成）または持ち込み。
- GUI はインターネットに直接公開せず、VPN 越しのアクセスを基本とする。SSH トンネルは前提にしない。
- CSRF 対策は `net/http` の `CrossOriginProtection` を使う。

## aka-only-server との接続

- 同一ホストの場合は、aka-only-server の compose が作る共有 Docker ネットワーク（既定の名前は `aka-av`）に外部ネットワークとして参加し、`https://aka-only-server:9443/admin/v1` に接続する。別ホストの場合も動くよう、接続先 URL は設定で与える。
- 管理API は mTLS 必須。BFF のクライアント証明書のフィンガープリントを、aka-only-server 側の `.env` の `AKA_ADMIN_CLIENTS` に `識別名=フィンガープリント` で登録する。
- 管理API のサーバー証明書は自己署名なので、BFF 側で検証用に設定する（`aka-only-server admin-cert` で取り出せる）。
- 証明書の作成とフィンガープリントの確認は、aka-only-server のコマンド（`client gen-cert`、`fingerprint`）でできる。

## 検証の進め方

- 単体テストに加えて、Valkey を使う結合テストは接続先を環境変数で指定したときだけ実行する形にする。aka-only-server 側は `AKA_TEST_VALKEY_ADDR` を使い、パッケージごとに論理データベースの番号を分けている。
- 通しの確認は、compose を別のプロジェクト名（`-p`）と専用の `.env`、ループバックの別ポートで起動して行い、終わったらコンテナ・ボリューム・イメージを片付ける。
- 実装した内容は、テストが通るだけでなく、実際に起動して操作して確かめる。確かめていない点は報告に明記する。
- ドキュメントに載せる手順（コマンド）は、実際に試してから載せる。

## 進め方

- コミットは依頼されたときだけ行う。
- 設計上の判断が要る点は、番号つきの確認事項として提示し、推奨案を添える。
- 実装が設計ドキュメントとずれた場合は、ドキュメントも同じ作業の中で更新する。
- フェーズ3の途中で管理API の変更が必要になったら、aka-only-server 側の OpenAPI・実装・テストも合わせて直す。
