module github.com/noi/dwbt/gui

go 1.25.0

require (
	github.com/noi/dwbt v0.0.0
	go.yaml.in/yaml/v3 v3.0.5
)

require github.com/expr-lang/expr v1.17.8 // indirect

replace github.com/noi/dwbt => ../
