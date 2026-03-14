package main

import (
	"fmt"
	"os"

	"github.com/Hideart/bepro/internal/version"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("bepro", version.Version)
		os.Exit(0)
	}

	fmt.Println("bepro", version.Version)
}
