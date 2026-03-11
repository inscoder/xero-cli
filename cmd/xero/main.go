package main

import (
	"github.com/inscoder/xero-cli/internal/commands"
	"github.com/inscoder/xero-cli/internal/version"
)

func main() {
	_ = commands.Execute(version.Version)
}
