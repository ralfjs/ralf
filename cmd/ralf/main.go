package main

import (
	"os"

	"github.com/Hideart/ralf/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
