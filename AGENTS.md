# AGENTS.md

AI エージェント向けの作業ガイド。プロジェクトの概要と使い方は `README.md`、
変換 API の仕様は `SPEC.md` を参照する。

## 作業ルール

- API の変更は `openapi.yaml`（OpenAPI 3.1）を編集し、`make generate` で再生成する。
  `internal/api/openapi.gen.go` は oapi-codegen v2.8.0 の生成物であり、手編集しない。
- 生成モデルやサーバーインターフェースだけでは保証されない検証
  （読みの正規化、全体と文節の読みの一致、書記素境界など）は
  `internal/httpapi` 側で実装する。
- ヘルスチェックのパスは `/healsh` とする（綴りは仕様どおり。`/health` に直さない）。
- ルート `/` に挨拶用エンドポイントを復活させない。
- 仕様を変更した場合は `SPEC.md` を、作業手順や進捗が変わった場合は本書を更新する。

## ビルドとテスト

Go 1.27.1（`mise.toml` で固定）を使う。

```sh
make generate          # Go モデルと Echo サーバーインターフェースを生成する。
make build             # bin/kkc-server をビルドする。
make test              # 単体テスト（実 Mozc のテストは下記プロファイルが必要）。
make mozc-up           # 専用 Mozc コンテナを build/start し、IPC と設定を確認する。
make test-integration  # 実 Mozc と HTTP 往復を含む全テストを実行する。
make mozc-down         # このワークスペースのコンテナを停止する（プロファイルは残す）。
make run               # Mozc を起動・確認してから API を 8080 で起動する。
```

- 変更後は少なくとも `make test` を通す。Mozc や HTTP の挙動に関わる変更では
  `make test-integration` も実行する。
- `make mozc-up` は繰り返し実行してよい。イメージ更新時は、このワークスペースが
  所有するコンテナだけを作り直す。`make clean` は API バイナリを削除するだけで
  Mozc は停止しない。
- 実 Mozc のテストは環境変数 `NETKKC_TEST_MOZC_PROFILE` でプロファイルを指定する
  （`make test-integration` が設定する）。

## Mozc の実行環境

- Mozc は Docker 内、Go の API は Linux ホスト上で動かす。
  ホストネットワークと共有プロファイルディレクトリで、Mozc の抽象 Unix ソケットと
  ディスカバリーファイルを公開する。TCP ブリッジは使わない。
- この構成は、ホストのネットワーク名前空間とバインドマウントのパスを Go プロセスと
  共有する Linux Docker Engine を前提とする。他プラットフォームの Docker Desktop は対象外。
- プロファイルは NetKKC 専用で、incognito・履歴不使用に設定する。
  起動時に Mozc の設定を書き換えるため、デスクトップ IME のプロファイルを指定しない。
- Make 変数 `MOZC_STATE_DIR`（絶対パス）、`MOZC_DOCKER_IMAGE`、`MOZC_DOCKER_CONTAINER`
  で上書きできる。外部で起動した専用 Mozc を使う場合は `make build` 後に
  バイナリを直接実行する（`-mozc-profile`、`-listen` を指定可能）。

## クラウド環境での注意

- クラウドイメージの `/usr/bin/go` は Go コンパイラではない。`PATH` 上の `go` が
  使えない場合、Make は `/workspace/.netkkc/toolchains/go/bin/go` を自動で使い、
  キャッシュを `/workspace/.netkkc` 配下に置く。シェルの activate は不要。
  別の Go を使う場合は `GO=/path/to/go` を Make に渡す。
- 既定のプロファイルは、クラウドでは `/workspace/.netkkc/docker-mozc/mozc`、
  それ以外のチェックアウトでは `.local/mozc/mozc`。
- Docker CLI の設定は `/workspace/.netkkc/docker-config` を使う。
- Docker build は既存のプロキシ設定（プロキシのホスト名解決を含む）を引き継ぐ。
  プロキシの認証情報を Dockerfile やイメージ設定に保存しない。
- クラウド環境では上記の Docker 構成で動作を確認済み。

## コード構成

| パス | 内容 |
| --- | --- |
| `main.go` | 起動処理とフラグ（`-listen`、`-mozc-profile`、`-check-mozc`）。 |
| `internal/api/` | oapi-codegen の生成コード。 |
| `internal/httpapi/` | HTTP ハンドラー、正規化・検証、エラーの対応付け。 |
| `internal/mozc/` | Mozc IPC クライアント（Unix ソケット＋protobuf）と変換処理。 |
| `scripts/mozc-docker.sh`, `docker/mozc.Dockerfile` | 専用 Mozc コンテナ。 |
| `.devcontainer/` | VS Code Dev Container。 |

`controller/`、`handler/`、`service/`、`example/`、`doc/` は初期構成の空ディレクトリ。

## 現在の実装状況

- Hello, World! のエンドポイントは撤去済み。
- `GET /healsh` と `POST /convert` を実装している。
- Go から Linux の専用 `mozc_server` に Unix ソケットと protobuf で接続する。
- 要求ごとに Mozc セッションを作成し、処理後に削除する。
- 専用プロファイルは incognito・履歴不使用に設定し、変換結果の学習は行わない。
- 文節境界は Mozc の文節伸縮で設定し、要求された境界と実際の結果の一致を確認する。
- すべての文節が確定済みなら、Mozc を呼ばずに確定候補を返す。
- サーバーは使える Mozc 接続を待ってから listen する。起動後に Mozc が使えなくなると
  変換は HTTP 503 を返し、ヘルスチェックは HTTP サーバーの稼働を返し続ける。
- 開発環境では `make run` が Docker 内の専用 Mozc を起動し、接続と設定を確認してから
  ホスト側の Go API を起動する。
- `make test-integration` はこの構成で実 Mozc と HTTP API を検証する。

## 今後の作業候補

- `SPEC.md` の「未確定事項・今後の検討事項」の具体化。
- VS Code Dev Container で API サーバーと `mozc_server` を同時に動かす構成の整備
  （単一 Dockerfile、Docker Compose は使わない）。
- デプロイ時に API サーバーと `mozc_server` を同一コンテナにまとめる構成の検討。
