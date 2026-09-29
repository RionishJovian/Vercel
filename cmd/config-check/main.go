// Command config-check validates configuration without starting services or
// opening database/network connections. Deploy automation uses it with
// ENVIRONMENT=production before replacing a running binary.
package main

import (
	"flag"
	"fmt"
	"os"

	"mega-magic/internal/config"
)

func main() {
	path := flag.String("config", "config.yaml", "configuration file to validate")
	flag.Parse()
	if _, err := config.Load(*path); err != nil {
		fmt.Fprintln(os.Stderr, "configuration invalid:", err)
		os.Exit(1)
	}
	fmt.Println("configuration valid")
}
