# 運用ガイド

- 対象: web-gui-for-aka-only-server（BFF + Web GUI）を導入・運用する人。
- 関連: [README](../README.md)、[設計概要](design-overview.md)、[画面仕様](screen-spec.md)、aka-only-server の運用ガイド（`../aka-only-server/docs/operation-guide.md`）

## 1. 構成

`docker compose` で 2 つのコンテナを動かす。

| コンテナ | 内容 |
|---|---|
| `aka-webgui` | BFF と Web GUI。ブラウザ向けの HTTPS を自身で終端し、aka-only-server の管理API を mTLS で呼ぶ |
| `valkey` | BFF 専用の Valkey。アカウント、セッション、BFF の監査ログを保存する。ホストにもほかのプロジェクトにもポートを公開しない |

| 待ち受け | コンテナ内のポート | ホスト側の公開先（既定） |
|---|---|---|
| ブラウザ向け HTTPS | 8443 | `127.0.0.1:8444`（`.env` の `WEBGUI_PUBLISH`） |

| データ | 保存先 |
|---|---|
| アカウント（パスワードは argon2id のハッシュ）、セッション、BFF の監査ログ | ボリューム `valkey-data` |
| 自動生成したブラウザ向けの自己署名証明書と秘密鍵 | ボリューム `webgui-data` |
| 管理API 用の証明書、持ち込みのブラウザ向け証明書 | ホストの `certs/`（読み取り専用でマウント） |
| 加入者、AVクライアント、証明書、サーバーログ | aka-only-server 側（BFF は保存しない） |

GUI はインターネットに直接公開せず、VPN 越しに使う。ホスト OS は Debian を主対象とし、必要なのは Docker Engine と compose プラグインだけである。

## 2. 導入

aka-only-server と同じホストで動かす場合の手順を示す。別のホストの場合は 2.6 を参照。

以下では、2 つのリポジトリを同じディレクトリに並べて置く。

```
~/aka-only-server/                  aka-only-server（起動済み）
~/web-gui-for-aka-only-server/      このリポジトリ
```

### 2.1 aka-only-server 側: BFF を管理クライアントとして登録する

aka-only-server のディレクトリで作業する。

BFF のクライアント証明書を作る。証明書と秘密鍵が 1 つの PEM にまとめて出力される。

```bash
docker compose run --rm --no-deps -T aka-only-server client gen-cert -name bff > bff.pem
```

```bash
chmod 600 bff.pem
```

フィンガープリントを調べる。

```bash
docker compose run --rm --no-deps -T aka-only-server fingerprint < bff.pem
```

aka-only-server の `.env` の `AKA_ADMIN_CLIENTS` に `bff=<フィンガープリント>` を書き、起動し直す。識別名（`bff`）は aka-only-server の監査ログに管理クライアントとして残る。

```bash
docker compose up -d
```

管理API 用のサーバー証明書を取り出す。

```bash
docker compose exec -T aka-only-server /aka-only-server admin-cert > admin.pem
```

### 2.2 BFF 側: 証明書を置く

BFF のディレクトリで作業する。`certs/` に、2.1 で作った 2 つのファイルを決まった名前で置く。

```bash
mkdir -p certs
```

```bash
cp ../aka-only-server/bff.pem certs/admin-client.pem
```

```bash
cp ../aka-only-server/admin.pem certs/admin-server.pem
```

コンテナは UID 65532 で動くので、ファイルの所有者をこの UID にして、パーミッションを 600 にする。

```bash
sudo chown 65532:65532 certs/*.pem
```

```bash
sudo chmod 600 certs/*.pem
```

### 2.3 BFF 側: `.env` を書く

```bash
cp .env.example .env
```

```bash
chmod 600 .env
```

少なくとも次の項目を書き換える。全項目は 10 章を参照。

| 項目 | 内容 |
|---|---|
| `VALKEY_PASSWORD` | BFF 専用の Valkey のパスワード |
| `WEBGUI_INITIAL_ADMIN_ID` / `WEBGUI_INITIAL_ADMIN_PASSWORD` | 最初の管理者（5 章） |
| `WEBGUI_PUBLISH` | ブラウザ向け HTTPS を公開する VPN 側のアドレスとポート（4 章）。例: `100.64.0.10:8444` |
| `WEBGUI_TLS_HOSTS` | 自己署名の証明書の SAN。ブラウザで開くときの名前や IP アドレス（3 章）。例: `100.64.0.10` |

### 2.4 起動する

aka-only-server が先に起動している必要がある（共有ネットワーク `aka-av` を aka-only-server の compose が作るため）。

```bash
docker compose up -d --build
```

管理API に接続できることを確かめる。

```bash
docker compose exec aka-webgui /aka-webgui check-admin
```

`接続できました。` と出れば準備は終わりである。接続できない場合は、原因の見当が表示される（9 章）。

### 2.5 ログインする

ブラウザで `https://<WEBGUI_PUBLISH のアドレス>:<ポート>/` を開き、最初の管理者でログインする。自己署名の証明書を使っている場合は、ブラウザの警告が出る（3.1）。

最初の管理者で、日常の操作に使う管理者や一般ユーザーのアカウントを作る（5 章）。

### 2.6 別のホストで動かす場合

BFF と aka-only-server を別のホストで動かす場合は、次のように変える。以下では、aka-only-server のホストの VPN 側のアドレスを `100.64.0.20` とする。

aka-only-server 側（aka-only-server のディレクトリ）:

1. `.env` の `AKA_ADMIN_PUBLISH` を `100.64.0.20:9443` にし、`AKA_ADMIN_TLS_HOSTS` に `100.64.0.20` を加える。
2. 新しい設定でコンテナを作り直す。`AKA_ADMIN_TLS_HOSTS` は動いているコンテナには反映されないので、証明書を作り直す前に行う。

   ```bash
   docker compose up -d
   ```

3. 管理API のサーバー証明書を作り直し、取り出す。

   ```bash
   docker compose exec -T aka-only-server /aka-only-server admin-cert reset > admin.pem
   ```

4. 起動し直して、新しい証明書を使わせる。

   ```bash
   docker compose restart aka-only-server
   ```

BFF 側（BFF のディレクトリ）:

1. `admin.pem` と `bff.pem` を BFF のホストに移し、2.2 の手順で `certs/` に置く。既に置いてあるファイルを置き換える場合は、所有者が UID 65532 なので `sudo cp` で上書きする。
2. `.env` の `COMPOSE_FILE` を `compose.yaml`（共有ネットワークを使わない）にし、`WEBGUI_ADMIN_URL` を `https://100.64.0.20:9443/admin/v1` にする。
3. 起動し直して、`check-admin` で確かめる。

   ```bash
   docker compose up -d
   ```

## 3. ブラウザ向けの HTTPS

BFF は HTTPS を自身で終端する。リバースプロキシは置かない。HSTS は付けない（自己署名の証明書で警告を越えて使えなくなるため）。

### 3.1 自己署名の証明書（既定）

初回起動時に、`.env` の `WEBGUI_TLS_HOSTS` を SAN に入れた自己署名の証明書（有効期間 10 年）を作り、ボリューム `webgui-data` に保存する。

- ブラウザは警告を出す。フィンガープリントを確かめてから進むか、証明書をブラウザ（または OS）の信頼済みの証明書に登録する。
- 起動ログの `loaded server certificate` に、証明書の SHA-256 フィンガープリントが出る。

```bash
docker compose logs aka-webgui | grep 'loaded server certificate'
```

`WEBGUI_TLS_HOSTS` は生成済みの証明書には反映されない。変えた場合は、保存した証明書を消して起動し直す。ボリュームの実際の名前は `<プロジェクト名>_webgui-data` で、プロジェクト名は既定では compose ファイルのあるディレクトリ名になる。

```bash
docker compose stop aka-webgui
```

```bash
docker run --rm -v web-gui-for-aka-only-server_webgui-data:/data valkey/valkey:9 rm /data/tls/cert.pem /data/tls/key.pem
```

```bash
docker compose start aka-webgui
```

### 3.2 Tailscale の証明書

tailnet で HTTPS 証明書を有効にしていれば、`tailscale cert` で Let's Encrypt の証明書を取り出せる。ブラウザの警告は出ない。ブラウザでは `https://<マシン名>.<tailnet 名>.ts.net:<ポート>/` で開く。

- tailnet で HTTPS 証明書を有効にすると、マシン名が Certificate Transparency のログに公開される。
- HTTPS 証明書を使うには、tailnet で MagicDNS が有効である必要がある。ホスト OS の DNS 設定と競合する場合は、tailnet の MagicDNS は有効のまま、端末ごとに `sudo tailscale set --accept-dns=false` とし、ブラウザを使う端末の hosts ファイルにマシン名と Tailscale のアドレスを書く。

証明書を `certs/` に取り出す（BFF のディレクトリで実行する。`<マシン名>.<tailnet 名>.ts.net` は `tailscale status --json` の `Self.DNSName` で確かめられる）。

```bash
sudo tailscale cert --cert-file certs/tls-cert.pem --key-file certs/tls-key.pem <マシン名>.<tailnet 名>.ts.net
```

```bash
sudo chown 65532:65532 certs/tls-cert.pem certs/tls-key.pem
```

`.env` で持ち込みの証明書を指定し、起動し直す。

```
WEBGUI_TLS_CERT=/certs/tls-cert.pem
WEBGUI_TLS_KEY=/certs/tls-key.pem
```

```bash
docker compose up -d
```

#### 証明書の更新

Let's Encrypt の証明書の有効期間は約 90 日である。`tailscale cert` は実行したときにファイルを書き出すだけなので、定期的に実行する。BFF はファイルが変わったことを 1 分以内に検知して読み直すので、再起動は要らない。

systemd のタイマーで毎日実行する例を示す。`/home/<ユーザー>/web-gui-for-aka-only-server` は置いた場所に合わせる。

`/etc/systemd/system/aka-webgui-cert.service`:

```ini
[Unit]
Description=Renew the Tailscale certificate for aka-webgui
After=tailscaled.service

[Service]
Type=oneshot
WorkingDirectory=/home/<ユーザー>/web-gui-for-aka-only-server
ExecStart=/usr/bin/tailscale cert --cert-file certs/tls-cert.pem --key-file certs/tls-key.pem <マシン名>.<tailnet 名>.ts.net
ExecStartPost=/usr/bin/chown 65532:65532 certs/tls-cert.pem certs/tls-key.pem
```

`/etc/systemd/system/aka-webgui-cert.timer`:

```ini
[Unit]
Description=Renew the Tailscale certificate for aka-webgui daily

[Timer]
OnCalendar=daily
RandomizedDelaySec=1h
Persistent=true

[Install]
WantedBy=timers.target
```

```bash
sudo systemctl daemon-reload
```

```bash
sudo systemctl enable --now aka-webgui-cert.timer
```

一度手で実行して、エラーにならないことを確かめる。

```bash
sudo systemctl start aka-webgui-cert.service
```

```bash
systemctl status aka-webgui-cert.service --no-pager
```

`tailscale cert` は、有効期限まで十分あるときは同じ証明書を書き出すので、毎日実行しても Let's Encrypt の発行回数の制限には当たらない。

### 3.3 WireGuard などの場合

自己署名の証明書（3.1）を使い、`WEBGUI_TLS_HOSTS` に VPN 側の IP アドレスを入れておく。ブラウザ側で証明書を信頼する設定をする。

## 4. 公開範囲

Docker が公開したポートは、ufw の規則を通らずに外部から届く。公開範囲は `.env` の `WEBGUI_PUBLISH` のバインド先アドレスで制御する。

- VPN 側のアドレス（Tailscale なら `tailscale ip -4` の値）を指定する。`0.0.0.0` にはしない。
- バインドできるのは起動時にホストにあるアドレスだけである。VPN のインターフェースが上がる前に Docker が起動すると、コンテナの起動に失敗する（`restart: unless-stopped` により再試行される）。

公開先を確かめる。

```bash
docker compose ps --format '{{.Name}} {{.Ports}}'
```

## 5. アカウントの運用

| 種類 | 作り方 | できること |
|---|---|---|
| 最初の管理者 | `.env` の `WEBGUI_INITIAL_ADMIN_ID` / `WEBGUI_INITIAL_ADMIN_PASSWORD` | 管理者と一般ユーザーの作成・削除・パスワード再設定、すべての操作 |
| 管理者 | 最初の管理者が作る | 一般ユーザーの作成・削除・パスワード再設定、Ki / OPc の表示と変更、AVクライアントと証明書の変更、監査ログ |
| 一般ユーザー | 管理者が作る | 加入者の登録・削除、許可クライアントと平文HTTP の変更、閲覧、ログ |

権限の詳細は[設計概要](design-overview.md)の 4 章を参照。

- 最初の管理者は Valkey に保存しない。パスワードを変えるときは `.env` を書き換えて `docker compose up -d` で起動し直す。最初の管理者のセッションは無効になる。
- 最初の管理者は、作業の初期設定と緊急時に使い、日常の操作は個別のアカウントで行う（aka-only-server の監査ログに操作者として残る）。
- アカウントを作ったり、パスワードを再設定したりすると、そのアカウントは次のログインでパスワードの変更を求められる。
- パスワードは 8 文字以上で、ユーザーID と同じものは使えない。

### 5.1 ログインのロック

同じユーザーID で `WEBGUI_LOGIN_MAX_FAILURES` 回（既定 5 回）続けて失敗すると、最初の失敗から `WEBGUI_LOGIN_LOCK_DURATION`（既定 15 分）が過ぎるまで、そのユーザーID ではログインできない。待たずに解除する場合は、BFF の Valkey から失敗回数を消す（`<ユーザーID>` を置き換える）。

```bash
docker compose exec valkey sh -c 'valkey-cli -a "$VALKEY_PASSWORD" --no-auth-warning del loginfail:<ユーザーID>'
```

### 5.2 セッション

- 操作がないまま `WEBGUI_SESSION_IDLE_TIMEOUT`（既定 30 分）過ぎるか、ログインから `WEBGUI_SESSION_MAX_AGE`（既定 12 時間）過ぎると、ログアウトする。
- ログの画面の自動更新では有効期限を延ばさない。
- パスワードの変更・再設定とアカウントの削除で、そのアカウントの他のセッションは無効になる。
- BFF を起動し直してもセッションは残る。

## 6. ログと監査ログ

| 種類 | 内容 | 見る場所 |
|---|---|---|
| BFF のログ | アクセスログ、管理API の呼び出し、エラー | `docker compose logs aka-webgui`（標準出力の JSON） |
| BFF の監査ログ | ログインの成功・失敗、アカウントの作成・削除、パスワードの再設定・変更 | 画面の「監査ログ」→「BFF」。標準出力にも出る |
| aka-only-server のログ | 払い出し、接続の拒否など | 画面の「ログ」 |
| aka-only-server の監査ログ | 管理API での変更操作と Ki / OPc の表示（操作者つき） | 画面の「監査ログ」→「aka-only-server」 |

```bash
docker compose logs -f aka-webgui
```

- BFF の監査ログは `WEBGUI_AUDIT_MAX` 件（既定 10000）を超えると古いものから消える。長く残したい場合は、Docker のログを外部に保存する。
- パスワード、Ki、OPc は BFF のログにも監査ログにも出ない。
- Docker のログは既定では無制限に増える。`/etc/docker/daemon.json` などでローテーションを設定しておく。

## 7. バックアップと復元

BFF のデータは 2 つのボリュームにある。加入者などのデータは aka-only-server 側にあるので、aka-only-server のバックアップも別に取る。

| ボリューム | 内容 | 失った場合 |
|---|---|---|
| `<プロジェクト名>_valkey-data` | アカウント、BFF の監査ログ | 最初の管理者以外のアカウントを作り直す |
| `<プロジェクト名>_webgui-data` | 自己署名の証明書と秘密鍵 | 起動時に作り直される（ブラウザの信頼設定をやり直す） |

以下はプロジェクト名が `web-gui-for-aka-only-server` の場合の例である。

### 7.1 バックアップ

```bash
docker compose stop
```

```bash
docker run --rm -v web-gui-for-aka-only-server_valkey-data:/data:ro -v "$PWD":/backup valkey/valkey:9 tar czf /backup/webgui-valkey-data.tgz -C /data .
```

```bash
docker compose start
```

バックアップのファイルは、コンテナの中で作るので root の所有（パーミッション 600）になる。移すときは `sudo` を使う。

### 7.2 復元

ボリュームを空にしてから、バックアップを展開する。現在のアカウントは消える。

```bash
docker compose down
```

```bash
docker run --rm -v web-gui-for-aka-only-server_valkey-data:/data -v "$PWD":/backup valkey/valkey:9 sh -c 'rm -rf /data/* && tar xzf /backup/webgui-valkey-data.tgz -C /data'
```

```bash
docker compose up -d
```

## 8. 更新

更新の前にバックアップを取っておく（7 章）。データはボリュームに残る。

```bash
git pull
```

設定項目が増えていないかを確かめる。`git pull` は更新前の位置を `ORIG_HEAD` に残すので、`.env.example` の変更を表示できる。何も表示されなければ、そのまま進む。

```bash
git diff ORIG_HEAD HEAD -- .env.example
```

増えた項目があれば、`.env` に書き足す。たとえば、`COMPOSE_FILE` は後から加わった項目で、これがない `.env` のまま更新すると、同一ホストでも共有ネットワークに参加せず、管理API に接続できなくなる（「ホスト名を解決できません」）。

ビルドし直して起動する。

```bash
docker compose up -d --build
```

```bash
docker compose exec aka-webgui /aka-webgui check-admin
```

## 9. 障害時の確認

まず `check-admin` で管理API への接続を確かめる。

```bash
docker compose exec aka-webgui /aka-webgui check-admin
```

| 症状・表示 | 確認すること |
|---|---|
| BFF が起動しない | `docker compose logs aka-webgui` の `error:`。最初の管理者の設定（未設定、パスワードが 8 文字未満、ユーザーID の形式）、Valkey のパスワード、`certs/` のファイルの有無と所有者（UID 65532） |
| `network aka-av declared as external, but could not be found` | aka-only-server が起動しているか。aka-only-server 側の `AKA_SHARED_NETWORK` と `.env` の `AKA_SHARED_NETWORK` が一致しているか。別ホストなら `COMPOSE_FILE=compose.yaml` にする |
| 「BFF のクライアント証明書を受け付けませんでした」 | aka-only-server 側の `AKA_ADMIN_CLIENTS` に、`check-admin` が表示するフィンガープリントが登録されているか |
| 「サーバー証明書が、設定した証明書と一致しません」 | `certs/admin-server.pem` が、aka-only-server の `admin-cert` で今取り出したものと同じか（`admin-cert reset` の後は取り出し直す） |
| 「サーバー証明書に、接続先のホスト名が入っていません」 | `WEBGUI_ADMIN_URL` のホスト名か IP アドレスが、aka-only-server 側の `AKA_ADMIN_TLS_HOSTS` に入っているか（2.6） |
| 「ホスト名を解決できません」 | aka-only-server が起動しているか。共有ネットワークに参加しているか（`COMPOSE_FILE`） |
| ログインできない（「一時的にログインできません」） | ログインのロック（5.1） |
| 画面が開けない | `WEBGUI_PUBLISH` のアドレスで待ち受けているか（4 章）、VPN がつながっているか |
| aka-only-server の `docker compose down` で `Network aka-av Resource is still in use` と出る | BFF が共有ネットワークに参加しているため、ネットワークが残っただけで、問題はない。aka-only-server を起動し直せば、BFF は再起動なしで接続し直す |

## 10. 環境変数

`.env` に書く。compose は `.env` の値をコンテナの環境変数として渡す。

| 変数 | 既定値 | 内容 |
|---|---|---|
| `COMPOSE_FILE` | `compose.yaml:compose.aka-av.yaml` | 別ホストなら `compose.yaml` |
| `AKA_SHARED_NETWORK` | `aka-av` | 同一ホストの aka-only-server が作る共有ネットワークの名前 |
| `WEBGUI_ADMIN_URL` | `https://aka-only-server:9443/admin/v1` | 管理API のベース URL |
| `VALKEY_PASSWORD` | （必須） | BFF 専用の Valkey のパスワード |
| `WEBGUI_INITIAL_ADMIN_ID` | （必須） | 最初の管理者のユーザーID |
| `WEBGUI_INITIAL_ADMIN_PASSWORD` | （必須） | 最初の管理者のパスワード（8 文字以上） |
| `WEBGUI_PUBLISH` | `127.0.0.1:8444` | ブラウザ向け HTTPS を公開するホスト側のアドレスとポート |
| `WEBGUI_TLS_HOSTS` | `localhost,127.0.0.1` | 自己署名の証明書の SAN |
| `WEBGUI_TLS_CERT` / `WEBGUI_TLS_KEY` | （空: 自己署名） | 持ち込みの証明書と秘密鍵のコンテナ内のパス（`/certs/...`） |
| `WEBGUI_SESSION_IDLE_TIMEOUT` | `30m` | 無操作でログアウトするまでの時間 |
| `WEBGUI_SESSION_MAX_AGE` | `12h` | ログインからログアウトするまでの最大時間 |
| `WEBGUI_LOGIN_MAX_FAILURES` | `5` | ログインをロックする失敗回数 |
| `WEBGUI_LOGIN_LOCK_DURATION` | `15m` | ロックする時間（最初の失敗から） |
| `WEBGUI_AUDIT_MAX` | `10000` | BFF の監査ログの保持件数 |
| `TZ` | `Asia/Tokyo` | 画面とログの日時のタイムゾーン |
| `WEBGUI_LOG_LEVEL` | `info` | ログのレベル（debug / info / warn / error） |
| `WEBGUI_VERSION` | `dev` | 画面に出すバージョン（ビルド時に埋め込む） |

コンテナの中では、このほかに `WEBGUI_ADDR`（`:8443`）、`WEBGUI_VALKEY_ADDR`（`valkey:6379`）、`WEBGUI_ADMIN_CLIENT_CERT`（`/certs/admin-client.pem`）、`WEBGUI_ADMIN_SERVER_CERT`（`/certs/admin-server.pem`）、`WEBGUI_ADMIN_CLIENT_KEY`（空: クライアント証明書のファイルから読む）を使う。compose を使わずに動かす場合は、これらも指定する。
