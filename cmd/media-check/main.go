// media-check is read-only. It never installs software, creates libraries or
// grants guest access. Credentials must not be supplied as command arguments.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"mega-magic/internal/media/jellyfin"
	"os"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	endpoint := os.Getenv("MM_JELLYFIN_LOCAL_URL")
	if endpoint == "" {
		endpoint = "http://127.0.0.1:8096"
	}
	client, err := jellyfin.NewLocal(endpoint, os.Getenv("MM_JELLYFIN_LOCAL_TOKEN"))
	if err != nil {
		return err
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	info, err := client.Status(ctx)
	if err != nil {
		return err
	}
	result := map[string]any{"server_name": info.ServerName, "version": info.Version, "setup_complete": info.StartupWizardCompleted, "connected_to_mega_magic": false}
	if os.Getenv("MM_JELLYFIN_LOCAL_TOKEN") != "" {
		libraries, err := client.Libraries(ctx)
		if err != nil {
			return err
		}
		result["library_count"] = len(libraries)
		result["libraries"] = libraries
	} else {
		result["library_check"] = "not performed: no local credential configured"
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
