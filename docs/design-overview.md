# web-gui-for-aka-only-server 設計概要

- 状態: ドラフト（2026-10-03 時点の検討結果）
- 対象: BFF と Web GUI。
- 関連: システム全体の構成と管理API は aka-only-server リポジトリの `docs/design-overview.md` を参照。

旧実装（Gin + HTMX + Alpine.js）は流用せず、コードとドキュメントを削除したうえで全面的に作り直す。

## 1. 位置づけ

aka-only-server の管理 GUI 実装の1つ。aka-only-server の管理API だけを使い、加入者データや鍵情報は自分では保存しない。

```
[ブラウザ] ──HTTPS（VPN 越し）──> BFF + Web GUI ──mTLS──> aka-only-server 管理API
                                      │
                                Valkey（BFF 専用）
```

- BFF は Go で実装し、HTML を返す（HTMX + Pico CSS）。
- ログインアカウント、権限、セッションはこちらで持つ。
- aka-only-server とは同一ホストでも別ホストでも動かせる。

## 2. 技術方針

- 言語: Go 1.27.1。
- 標準ライブラリを優先する。HTTP は `net/http`、テンプレートは `html/template`、静的ファイルは `embed`、ログは `log/slog`。
- HTMX と Pico CSS はファイルをリポジトリに同梱して配信する。Alpine.js は使わない。
- 外部依存は `github.com/valkey-io/valkey-go`（Valkey クライアント）と `golang.org/x/crypto`（argon2id）に限る。
- module パス: `github.com/oyaguma3/web-gui-for-aka-only-server`
- 配備: Docker Compose（BFF + 専用 Valkey）。

## 3. ブラウザ向けの HTTPS とネットワーク

- GUI はインターネットに直接公開せず、VPN 越しのアクセスを基本とする。SSH トンネルは前提にしない。
- HTTPS は BFF 自身（Go）で終端する。リバースプロキシは置かない。
- サーバー証明書は自己署名または持ち込みとする。初回起動時に証明書がなければ自己署名を自動生成する。
  - Tailscale を使う場合は `tailscale cert` で発行した証明書を持ち込めば、ブラウザの警告が出ない。
  - WireGuard などの場合は自己署名を使い、ブラウザ側で信頼設定をする。
- 待ち受けアドレスは設定で指定する。Docker が公開したポートは ufw の規則を通らないため、`ports` のバインド先を VPN 側のアドレスに限定する。

## 4. アカウントと権限

### 4.1 アカウント種別

| 種別 | 定義 |
|---|---|
| 最初の管理者 | `.env` で指定する。`.env` を常に正とし、Valkey には保存しない。削除不可。パスワード変更は `.env` の編集で行う |
| 管理者 | 最初の管理者だけが作成・削除できる |
| 一般ユーザー | 管理者が作成・削除できる |

- 最初の管理者と同じユーザーID のアカウントは作成できない。
- 最初の管理者のパスワードは `.env` に平文で書き、`.env` のパーミッションを 600 にする。

### 4.2 操作権限

| 操作 | 最初の管理者 | 管理者 | 一般ユーザー |
|---|---|---|---|
| 加入者の一覧・閲覧（Ki / OPc 以外） | ○ | ○ | ○ |
| 加入者の登録（Ki / OPc / SQN / AMF を入力） | ○ | ○ | ○ |
| 登録済み加入者の Ki / OPc 閲覧 | ○ | ○ | × |
| 登録済み加入者の Ki / OPc 変更 | ○ | ○ | × |
| 登録済み加入者の SQN / AMF 変更 | ○ | ○ | × |
| 登録済み加入者の SQN 増加タイプ変更 | ○ | ○ | × |
| 許可クライアント・平文HTTP許可の変更 | ○ | ○ | ○ |
| 加入者の削除 | ○ | ○ | ○ |
| ログ閲覧 | ○ | ○ | ○ |
| 監査ログ閲覧 | ○ | ○ | × |
| AVクライアント証明書の登録・削除 | ○ | ○ | × |
| AV用サーバー証明書の登録・削除 | ○ | ○ | × |
| 一般ユーザーアカウントの作成・削除 | ○ | ○ | × |
| 管理者アカウントの作成・削除 | ○ | × | × |
| 自分のパスワード変更 | ×（`.env`） | ○ | ○ |

- 一般ユーザーは登録フォームの入力中に限り Ki / OPc / SQN / AMF を閲覧・編集できる。登録が完了した後は Ki / OPc を表示しない。
- 一般ユーザーは加入者を削除して再登録することで、SQN / AMF を実質的に変更できる。これは許容し、登録・削除を操作者つきで aka-only-server の監査ログに残すことで追跡できるようにする。
- 権限判定は BFF だけで行う。aka-only-server は BFF を全権の管理クライアントとして扱う。
- BFF は操作者のユーザーID をリクエストヘッダーで管理API に渡し、サーバー側の監査ログに残す。

## 5. セッションとセキュリティ

- パスワードは argon2id でハッシュ化して保存する。
- ログインの成功・失敗、アカウントの作成・削除、パスワード再設定は、BFF 側の監査ログとして記録する。標準出力に加えて BFF 専用 Valkey の Stream に保存する。
- セッションはサーバー側（Valkey）に保持し、Cookie には ID だけを入れる。Cookie は HttpOnly / Secure / SameSite を付ける。
- CSRF 対策は `net/http` の `CrossOriginProtection` を使う。
- ログイン試行回数を制限する。
- Ki / OPc は BFF のログに出さない。

## 6. 画面

| 画面 | 内容 |
|---|---|
| ログイン | ユーザーID / パスワード |
| ダッシュボード | 加入者数、クライアント数、サーバー証明書の期限、直近のログ |
| 加入者一覧 | ページング、IMSI 前方一致検索 |
| 加入者登録 | IMSI、Ki、OPc、SQN、AMF、SQN 増加タイプ、許可クライアント、平文HTTP許可 |
| 加入者詳細・編集 | 権限に応じて項目を出し分ける |
| AVクライアント | 一覧、証明書 PEM の登録、有効/無効、証明書の差し替え、削除 |
| AV用サーバー証明書 | 現在の証明書の表示、登録、削除 |
| ログ | HTMX のポーリングで追記表示 |
| 監査ログ | aka-only-server の監査ログと BFF の監査ログを表示 |
| アカウント管理 | 一覧、作成、削除、パスワード再設定 |

## 7. 管理API との連携

- aka-only-server の `docs/openapi/admin-api.yaml` を契約とする。
- 接続先 URL、BFF のクライアント証明書と秘密鍵、管理API のサーバー証明書（検証用）を設定で与える。
- BFF のクライアント証明書は、フィンガープリントを aka-only-server 側の `.env`（`AKA_ADMIN_CLIENTS`）に書いて登録する。
- 同一ホストで動かす場合は、aka-only-server が作る共有の Docker ネットワークに参加し、`https://aka-only-server:9443` で接続する。別ホストの場合は、aka-only-server 側で管理API の公開先アドレスを変える。
- 手順は aka-only-server の `docs/operation-guide.md` を参照。

## 8. データモデル（BFF 専用 Valkey）

| キー | 型 | 内容 |
|---|---|---|
| `user:{id}` | Hash | `role`（admin / user）、`password_hash`、`created_at`、`created_by` |
| `users` | Set | ユーザーID の索引 |
| `session:{sid}` | Hash | `user_id`、`created_at`。有効期限つき |
| `loginfail:{id}` | String | ログイン失敗回数。有効期限つき |
| `audit` | Stream | BFF の監査ログ。件数上限つき |

## 9. 作成予定のドキュメント

| ファイル | 内容 |
|---|---|
| `docs/design-overview.md` | 本書 |
| `docs/screen-spec.md` | 画面仕様と権限ごとの表示差 |
| `docs/operation-guide.md` | 導入、アカウント運用。ブラウザ向け HTTPS の運用補助情報（Tailscale / WireGuard ごとの証明書の用意、Docker の公開ポートと ufw の関係、バインド先の限定）も記載する |
