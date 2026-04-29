package main

import (
	"github.com/butvinm/yandex-cli/internal/cli"
)

var version = "dev"

func main() {
	cli.Main(version)
}
