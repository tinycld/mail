package mail

import (
	"crypto/tls"
	"fmt"
	"os"
)

// loadTLSConfig loads a TLS certificate from the given environment variable names.
// It tries the primary env vars first, then the fallback env vars.
// Returns (nil, nil) if no cert paths are configured — TLS is optional in dev.
func loadTLSConfig(certEnv, keyEnv, fallbackCertEnv, fallbackKeyEnv string) (*tls.Config, error) {
	certPath := os.Getenv(certEnv)
	keyPath := os.Getenv(keyEnv)
	if certPath == "" && fallbackCertEnv != "" {
		certPath = os.Getenv(fallbackCertEnv)
	}
	if keyPath == "" && fallbackKeyEnv != "" {
		keyPath = os.Getenv(fallbackKeyEnv)
	}
	if certPath == "" || keyPath == "" {
		return nil, nil
	}

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS cert (%s/%s): %w", certEnv, keyEnv, err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		CipherSuites: []uint16{
			// TLS 1.2 AEAD-only ciphers (TLS 1.3 suites are automatic in Go)
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
		},
	}, nil
}
