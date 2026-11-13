package main

import (
	_ "embed"

	"github.com/innovativebioresearch/innbc-chain/command/root"
	"github.com/innovativebioresearch/innbc-chain/licenses"
)

var (
	//go:embed LICENSE
	license string
)

func main() {
	licenses.SetLicense(license)

	root.NewRootCommand().Execute()
}
