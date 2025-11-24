# はじめに
このWebサーバーは、下記の設計ドキュメントを Google Antigravity (Gemini 3 pro - High) にプロンプトとして投入して作られたものです。別リポジトリのaka only serverと同様に、Google Antigravityのトライアルが目的です。  
一応、Windows 11上でaka only serverを起動した上で、このWeb GUI自体の正常系動作は確認しましたが、実用に値するかは保証しません。  
なお、こちらもユーザーガイド(USER_GUIDE.md)を出力させているので、本当に使ってみたい方はこれらを参照してみてください。  

-----
# アプリ設計ドキュメント

### ■アプリケーションのコンセプト

「AKA-Only-Server」というAPIサーバーのバックエンドにある、加入者データベースの情報を登録・閲覧・更新・削除するためのWeb GUIクライアントを開発する。  
「AKA-Only-Server」とは、指定したIMSIに対して3G AKAの認証ベクターを返すAPIサーバーであり、そのバックエンドに加入者データベースを持っている。この加入者データベースに対する加入者情報の登録・閲覧・更新・削除については、「AKA-Only-Server」自体にAPIエンドポイントとそのAPIが規定されているため、これらのAPIを用いて操作を行う。

------

### ■使用言語・ライブラリなど

- 開発言語として Go を使用する。
    - 環境変数ファイルを取り扱うため、パッケージ joho/godotenv を利用すること。
    - 機能実装に必要ならば、Webフレームワークとして Gin を利用すること。
- Web GUIを構成するためのコンポーネント・モジュールとして、以下を採用する。
    - htmx
    - Alpine.js

- node.jsやTailwind CSSなど、npmインストールなどによる環境構築を要するコンポーネント・モジュールは、可能な限り不採用とする。

### ■参照すべき仕様

#### （加入者データベース）

- 「AKA-Only-Server」の加入者データベースは、以下のSQLコマンドで作成されているものとする。

    ```sql
    CREATE DATABASE akaserverdb;
    \c akaserverdb
    CREATE TABLE public.subscribers (
        imsi VARCHAR(15) PRIMARY KEY,
        ki   VARCHAR(32) NOT NULL,
        opc  VARCHAR(32) NOT NULL,
        sqn  VARCHAR(12) NOT NULL,
        amf  VARCHAR(4)  NOT NULL,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
        CONSTRAINT chk_imsi_format CHECK (imsi ~ '^[0-9]{15}$'),
        CONSTRAINT chk_ki_hex      CHECK (ki  ~ '^[0-9a-fA-F]{32}$'),
        CONSTRAINT chk_opc_hex     CHECK (opc ~ '^[0-9a-fA-F]{32}$'),
        CONSTRAINT chk_sqn_hex     CHECK (sqn ~ '^[0-9a-fA-F]{12}$'),
        CONSTRAINT chk_amf_hex     CHECK (amf ~ '^[0-9a-fA-F]{4}$')
    );
    CREATE USER akaserver WITH PASSWORD 'akaserver';
    GRANT CONNECT ON DATABASE akaserverdb TO akaserver;
    GRANT USAGE ON SCHEMA public TO akaserver;
    GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO akaserver;
    ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO akaserver;
    ```


#### （利用するAPI）

- 「AKA-Only-Server」のAPI仕様は以下のドキュメントを参照すること。ただし、認証ベクター取得APIおよび再同期処理用APIは利用しない。
    - https://github.com/oyaguma3/aka-only-server/blob/main/docs/api_spec.md

### ■要件

#### （アプリ本体）

- アプリ本体が単一の実行バイナリーとなるようコンパイルできること。
- このアプリは「AKA-Only-Server」に対してはAPIクライアントとして振る舞い、Web GUIからはWebサーバーとして振る舞う。
- 環境変数のファイル名は webgui.env とする。
- Web GUIに対するWebサーバーとして待ち受けるためのIPアドレス＆ポート番号は、環境変数ファイルで指定可能とする。
    - デフォルトは localhost:9999 とする。
- 「AKA-Only-Server」に対するAPIクライアントとなる際の、接続先ホストのIPアドレス＆ポート番号は、環境変数ファイルで指定可能とする。
    - デフォルトは localhost:8080 とする。
- 実行時に systemd にサービスとして登録され、systemctl start/stop/restart などのコマンドで操作可能とする。
    - これは、アプリをバックグラウンドで稼働させることを主要な目的とする。
- バックエンドサーバーでも、入力される加入者情報の各パラメーターに対するバリデーションチェックを行うこと。

#### （アプリ利用者の認証とセッション管理）

- アプリ利用者の認証は、単純な username / password 一致確認で行う。
    - username / password のセットは、環境変数ファイルで指定可能とする。
    - デフォルトの username / password セットは「admin / admin」とする。
- Cookieを用いたセッション管理を行うこと。
    - セッションの有効期間はデフォルトで24時間とし、環境変数ファイルにより分単位で指定可能とする。

#### （Web GUI）

- 画面タイトルは「AKA-Only-Server Web GUI」とする。
- 利用者認証のためのログイン画面で利用者認証を行ったのち、加入者情報管理画面へと遷移する。
- GUIそのものは、利用するコンポーネント・モジュールの特徴と特性を活かして、シンプルかつ軽量に構成すること。
    - htmxとAlpine.jsは、CDNを使わずローカルで完結して動作させるものとする。
- PC上から一般的なWebブラウザ（Chromeやfirefoxなど）でアクセスされることを想定したデザインとする。
- カラーパターンは、白を基調とした一般的なものを利用する。
- 表示する加入者情報の数が100を超える場合、登録件数が100件以上あることについての警告と具体的な登録件数を表示し、利用者に表示処理を進めてよいか確認を行うこと。
- 入力値に対するバリデーションチェックを実施すること。
    - 「AKA-Only-Server」の加入者データベースへの格納を想定したバリデーションチェックであること。

#### （ログファイル）

- 標準ライブラリーの log/slog を利用し、出力ログを構造化すること。
- 出力ログのローテーション機能を具備すること。
    - 利用するGoパッケージとして natefinch/lumberjack を推奨する。
- 環境変数ファイルでログ出力先やローテーションの各種設定を指定できること。
    - デフォルトのログ出力先は、実行バイナリーと同じ場所とする。

### ■その他

- アプリ利用のためのユーザーガイドを、Markdown形式のドキュメントとして作成すること。
