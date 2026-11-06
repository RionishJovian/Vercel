// magic-discover listens for nearby MikroTik routers and prints what it hears as JSON.
// It only receives: it never connects to a router, sends a login, or changes configuration.
// Every result is an unverified observation, not proof of ownership, connection or online state.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"mega-magic/internal/discovery"
)

func main() {
	window := flag.Duration("window", 40*time.Second, "how long to listen (5s-2m); routers announce about every 30s by default")
	flag.Parse()
	if *window < 5*time.Second || *window > 2*time.Minute {
		fmt.Fprintln(os.Stderr, "window must be between 5s and 2m")
		os.Exit(2)
	}
	ad := discovery.MikroTik{}
	conn, err := discovery.OpenUDP(ad)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not listen on UDP %d: %v\nAnother program (for example WinBox) may be using it, or the system blocked local-network access.\n", ad.Port(), err)
		os.Exit(1)
	}
	defer conn.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	fmt.Fprintf(os.Stderr, "Listening %s for MikroTik announcements. Nothing is sent or changed.\n", *window)
	res, err := discovery.Listen(ctx, conn, ad, *window, time.Now)
	out := struct {
		Adapter discovery.Capabilities `json:"adapter"`
		discovery.Result
		Note string `json:"note"`
	}{ad.Capabilities(), res, "Observations are unverified. Hearing nothing does not mean no router is nearby."}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
	if err != nil {
		fmt.Fprintln(os.Stderr, "listen error:", err)
		os.Exit(1)
	}
}
