# users example

ユーザーの作成とフォローを行う API をテストするサンプルです。

| パス | 内容 |
|---|---|
| `.dwbt/config.yaml` | 環境プロファイル（`local` / `ci`） |
| `.dwbt/actions/user/` | ユーザー定義アクション（`user/create`、`user/follow`） |
| `.dwbt/workflows/` | ワークフロー |
| `mockapi/` | テスト対象のモック API の実装 |
| `server/` | モック API を起動するコマンド |

## 実行方法

コマンドはすべて、このディレクトリ（`examples/users`）で実行します。ターミナルを 2 つ使います。

1 つ目のターミナルで、モック API を起動します（`127.0.0.1:8080` で待ち受けます）。

```sh
go run ./server
```

2 つ目のターミナルで、ワークフローを実行します。

```sh
go run ../../cmd/dwbt run
```

> 1 つのコマンドで `go run ./server & ...` と続けて実行すると、サーバーの起動（コンパイル）が終わる前に dwbt がリクエストを送ってしまい、接続エラーになることがあります。

### 接続先を変える

```sh
go run ./server -addr 127.0.0.1:18080
go run ../../cmd/dwbt run --server api=http://127.0.0.1:18080

# ci プロファイルは環境変数 API_URL から接続先を読みます
API_URL=http://127.0.0.1:18080 go run ../../cmd/dwbt run --env ci
```

### 定義の検証だけ行う

```sh
go run ../../cmd/dwbt validate
```
