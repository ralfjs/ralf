package main

import (
	"os"

	"github.com/ralfjs/ralf/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
