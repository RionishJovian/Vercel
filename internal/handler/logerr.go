package handler

import (
	"fmt"
	"log"
)

// logIfErr is for a failure that must not fail the request but must not vanish either:
// the request goes on, and the failure is in the log with what it was about.
// Never pass secrets in the description.
func logIfErr(err error, what string, args ...any) {
	if err != nil {
		log.Printf("%s: %v", fmt.Sprintf(what, args...), err)
	}
}
