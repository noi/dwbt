# dwbt

dwbt (Defined Workflow Based Tests) は、E2E テストを YAML のワークフローとして記述し、実行するツールです。

仕様は [Issue #1 の仕様コメント](https://github.com/noi/dwbt/issues/1) を参照してください。

## インストール

```sh
go install github.com/noi/dwbt/cmd/dwbt@latest
```

## 使い方

```text
dwbt validate [workflow...]
dwbt run      [workflow...] [--env <name>] [--server <id>=<url>]...
```

カレントディレクトリから親へさかのぼって `.dwbt` ディレクトリを探し、その中の定義を読み込みます。

```text
.dwbt/
  config.yaml          # 環境プロファイル
  actions/
    user/create.yaml   # → use: user/create
  workflows/
    follow.yaml
```

終了コードは、成功が `0`、`expects` の不一致が `1`、それ以外のエラーが `2` です。

[examples/users](examples/users) にサンプルがあります。

## Go によるアクションの拡張

dwbt をライブラリとして使い、Go で実装したアクションを登録したバイナリをビルドできます。

```go
package main

import "github.com/noi/dwbt"

func main() {
	dwbt.New().Action("db/seed", seedAction{}).Main()
}
```

アクションは [`action.Action`](action/action.go) インターフェースを実装します。
