# users example

ユーザーの作成とフォローを行う API をテストするサンプルです。

| パス | 内容 |
|---|---|
| `.dwbt/config.yaml` | 環境プロファイル（`local` / `ci`） |
| `.dwbt/actions/user/` | ユーザー定義アクション（`user/create`、`user/follow`、`user/delete`） |
| `.dwbt/workflows/` | ワークフロー |
| `mockapi/` | テスト対象のモック API の実装 |
| `server/` | モック API を起動するコマンド |
| `users_test.go` | `dwbttest` でワークフローを実行するテスト |

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

# Unix ドメインソケットで待ち受けます
go run ./server -unix /tmp/users.sock
go run ../../cmd/dwbt run --server api=unix:///tmp/users.sock
```

### 定義の検証だけ行う

```sh
go run ../../cmd/dwbt validate
```

### Go のテストから実行する

`users_test.go` は、モック API を `httptest` で起動し、`dwbttest` でワークフローを実行します。サーバーを別に起動する必要はありません。

```sh
go test .
go test . -run TestWorkflows/follow.yaml -v
```
