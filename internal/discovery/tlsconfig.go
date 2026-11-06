package discovery

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"os"
)

// ClientWithExtraCA returns an HTTPS client that trusts the system's certificate authorities plus the ones in a PEM file.
// It exists so the helper can be tried against a test server with its own local certificate authority, without turning
// certificate checking off: verification stays on, and only that one extra authority is added. It is opt-in (--ca-file).
func ClientWithExtraCA(path string) (*http.Client, error) {
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pemBytes) {
		return nil, errors.New("no certificate found in the CA file")
	}
	return &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}}, nil
}
