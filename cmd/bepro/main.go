package main

import (
	"os"

	"github.com/Hideart/bepro/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
