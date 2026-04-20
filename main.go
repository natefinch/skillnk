package main

import (
	"fmt"
	"os"

	"github.com/natefinch/skillnk/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "skillnk:", err)
		os.Exit(1)
	}
}
