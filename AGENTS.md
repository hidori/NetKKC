# AGENTS.md

AI エージェント向けの作業ガイド。プロジェクトの概要と使い方は `README.md`、
変換 API の仕様は `doc/SPEC.md` を参照する。

## 作業ルール

- 初期段階は必要最小限で作る。頼まれていない検証・正規化・防御的処理・周辺スクリプトを
  先回りで入れない。追加したいものは提案に留め、判断を仰ぐ。
- 想定クライアントは IoT・ホビー用途のマイコン（ESP32、Raspberry Pi Pico W など）。
  クライアントに Unicode 正規化や書記素分割などの重い処理を要求しない。
- API の変更は `openapi.yaml`（OpenAPI 3.1）を編集し、`make generate` で再生成する。
  `internal/api/openapi.gen.go` は oapi-codegen v2.8.0 の生成物であり、手編集しない。
- 生成モデルやサーバーインターフェースだけでは保証されない検証
  （全体と文節の読みの一致など）は `internal/httpapi` 側で実装する。
- ヘルスチェックのパスは `/healsh` とする（綴りは仕様どおり。`/health` に直さない）。
- 仕様を変更した場合は `doc/SPEC.md` を、作業手順や進捗が変わった場合は本書を更新する。

## 開発環境

VS Code Dev Container で開く。`compose.yaml` が次の 3 サービスを起動する。
3 つは `mozc` のネットワーク名前空間を共有する（Mozc の抽象 Unix ソケットを共有するため）。

| サービス | 内容 |
| --- | --- |
| `dev` | 開発用コンテナ。ビルド・テストはここで行う。 |
| `api` | air によるホットリロードで API を起動する。ワークスペースは読み取り専用。 |
| `mozc` | 専用の `mozc_server`。プロファイルは `.local/mozc`。 |

- API のポートは名前空間の持ち主である `mozc` 側で `127.0.0.1:8080` に公開する。
- Mozc のディスカバリーファイル（`.session.ipc`）は所有者しか読めないため、
  `mozc` と `api` は `.devcontainer/run-as-workspace-owner.sh` でワークスペースの所有者
  （uid/gid）として動く。`dev` は `updateRemoteUserUID` でホストの uid に合わせる。
- Mozc のバージョンは `docker/mozc.Dockerfile` で固定している。

## ビルドとテスト

`dev` コンテナで実行する。Go 1.27.1（`mise.toml` で固定）を使う。

```sh
make generate          # Go モデルと Echo サーバーインターフェースを生成する。
make build             # bin/kkc-server をビルドする。
make test              # 全テスト（dev では実 Mozc のテストも含む）。
make test-integration  # テストキャッシュを使わずに全テストを実行する。
```

- 実 Mozc のテストは環境変数 `NETKKC_TEST_MOZC_PROFILE` が設定されているときだけ動く
  （`dev` では compose が設定する）。未設定ならスキップされる。
- クラウド環境では、`PATH` 上の `go` が使えない場合に Make が
  `/workspace/.netkkc/toolchains/go/bin/go` を使う。別の Go は `GO=/path/to/go` で指定する。

## コード構成

| パス | 内容 |
| --- | --- |
| `main.go` | 起動処理とフラグ（`-listen`、`-mozc-profile`。既定は環境変数 `MOZC_PROFILE_DIR`）。 |
| `internal/api/` | oapi-codegen の生成コード。 |
| `internal/httpapi/` | HTTP ハンドラー、要求の検証、エラーの対応付け。 |
| `internal/mozc/` | Mozc IPC クライアント（Unix ソケット＋protobuf）と変換処理。 |
| `compose.yaml`, `.devcontainer/` | 開発環境。 |
| `docker/mozc.Dockerfile` | Mozc イメージ。 |

## 現在の実装状況

- `GET /healsh` と `POST /convert` を実装している。
- 読みは加工せずに Mozc に渡す。応答の読みは Mozc が返した読み
  （全角英数字は半角に書き換えられる）。
- 要求ごとに Mozc セッションを作成し、処理後に削除する。
  専用プロファイルは incognito・履歴不使用で、学習は行わない。
- Mozc の IPC には文字列を直接変換する命令や文節境界を指定する命令がないため、
  キー操作（入力→SPACE、SHIFT+←/→ で文節伸縮）で変換と境界の再現を行い、
  結果の文節の読みが指定どおりかを検証する。再現できなければ 422。
- すべての文節が確定済みなら、Mozc を呼ばずに確定候補を返す。
- サーバーは Mozc の設定が完了するまで最大 10 秒待ってから listen する。

## 今後の作業候補

- `doc/SPEC.md` の「未確定事項・今後の検討事項」の具体化。
  特に候補数の上限は、マイコンの RAM を考えると早めに決める価値がある
  （実測で 1 文節 200 件を超える場合がある）。
- デプロイ時に API サーバーと `mozc_server` を同一コンテナにまとめる構成の検討。
