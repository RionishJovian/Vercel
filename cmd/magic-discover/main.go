// magic-discover listens for nearby MikroTik routers and reports what it hears.
// It only receives on the local network: it never connects to a router, sends a login, or changes configuration.
// Every result is an unverified observation, not proof of ownership, connection or online state.
//
//	magic-discover                      listen and print the results here
//	magic-discover --dry-run            also print the exact data that would be sent to Mega Magic, and send nothing
//	magic-discover --pair ABCDEF-GHJKMNPQ   pair with the code shown in Mega Magic and upload what it hears
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"mega-magic/internal/discovery"
)

const uploadEvery = 15 * time.Second

func main() {
	window := flag.Duration("window", 40*time.Second, "how long to listen (5s-2m); routers announce about every 30s by default")
	pair := flag.String("pair", "", "pairing code shown in Mega Magic; uploads what is heard to your own account")
	server := flag.String("server", "https://megamagic.tech", "Mega Magic address (https only)")
	caFile := flag.String("ca-file", "", "PEM file with one extra certificate authority to trust, for testing against a server with its own local certificate (certificate checking stays on)")
	dry := flag.Bool("dry-run", false, "print exactly what would be uploaded and send nothing")
	flag.Parse()
	if *window < 5*time.Second || *window > 2*time.Minute {
		fmt.Fprintln(os.Stderr, "window must be between 5s and 2m")
		os.Exit(2)
	}
	if *pair != "" && *dry {
		fmt.Fprintln(os.Stderr, "use either --pair or --dry-run, not both")
		os.Exit(2)
	}
	var up *discovery.Uploader
	if *pair != "" {
		var err error
		var hc *http.Client
		if *caFile != "" {
			if hc, err = discovery.ClientWithExtraCA(*caFile); err != nil {
				fmt.Fprintln(os.Stderr, "cannot use the CA file:", err)
				os.Exit(2)
			}
		}
		if up, err = discovery.NewUploader(*server, hc); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
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

	var session discovery.PairedSession
	var report func(discovery.Result) error
	if up != nil {
		if session, err = up.Pair(ctx, *pair); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "Paired. Uploading what is heard to your Mega Magic account every 15 seconds. Press Ctrl+C to stop.")
		report = func(r discovery.Result) error {
			_, _, err := up.Upload(ctx, session, r.Observations)
			if errors.Is(err, discovery.ErrSlowDown) {
				return nil
			}
			return err
		}
	}
	fmt.Fprintf(os.Stderr, "Listening %s for MikroTik announcements. Nothing is sent to any router.\n", *window)
	res, err := discovery.ListenProgress(ctx, conn, ad, *window, time.Now, uploadEvery, report)
	if err == nil && report != nil {
		err = report(res) // the final batch
	}
	if errors.Is(err, discovery.ErrSessionEnded) {
		fmt.Fprintln(os.Stderr, "The discovery session has ended in Mega Magic, so uploading stopped.")
		err = nil
	}
	out := struct {
		Adapter discovery.Capabilities `json:"adapter"`
		discovery.Result
		Note string `json:"note"`
	}{ad.Capabilities(), res, "Observations are unverified. Hearing nothing does not mean no router is nearby."}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
	if *dry {
		payload, _ := discovery.Payload(res.Observations)
		fmt.Fprintf(os.Stderr, "\nDry run: this is exactly what would be uploaded (nothing was sent):\n%s\n", payload)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
