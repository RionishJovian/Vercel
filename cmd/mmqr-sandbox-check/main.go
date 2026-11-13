// mmqr-sandbox-check exercises provider test credentials without starting the app.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"mega-magic/internal/payments/mmpay"
	"os"
	"time"
)

func main() {
	order := flag.String("order", "", "stable sandbox order ID")
	amount := flag.Int64("amount", 0, "whole MMK; required with -create")
	create := flag.Bool("create", false, "create one sandbox request; default only inquires")
	flag.Parse()
	client, err := mmpay.NewSandbox(mmpay.Config{BaseURL: os.Getenv("MMPAY_SANDBOX_BASE_URL"), AppID: os.Getenv("MMPAY_SANDBOX_APP_ID"), PublishableKey: os.Getenv("MMPAY_SANDBOX_PUBLISHABLE_KEY"), SecretKey: os.Getenv("MMPAY_SANDBOX_SECRET_KEY")})
	if err != nil {
		fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var p mmpay.Payment
	if *create {
		p, err = client.Create(ctx, *order, *amount, os.Getenv("MMPAY_SANDBOX_CALLBACK_URL"))
	} else {
		p, err = client.Get(ctx, *order)
	}
	if err != nil {
		fail(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(struct {
		Mode         string        `json:"mode"`
		Payment      mmpay.Payment `json:"payment"`
		GrantsAccess bool          `json:"grants_access"`
	}{"sandbox", p, false}); err != nil {
		fail(err)
	}
}

func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
