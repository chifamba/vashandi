package tls

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"golang.org/x/crypto/acme"
	"github.com/smallstep/certificates/acme/autocert"
)

// Config represents the internal TLS configuration
type Config struct {
	Enabled     bool
	Enforced    bool
	CAURL       string
	Fingerprint string
	CertPath    string
	KeyPath     string
	RootCAPath  string
	DNSNames    []string
}

// LoadConfigFromEnv loads mTLS configuration from environment variables
func LoadConfigFromEnv() Config {
	enabled := os.Getenv("MTLS_ENABLED") == "true"
	enforced := os.Getenv("MTLS_ENFORCED") == "true"
	caURL := os.Getenv("STEP_CA_URL")
	fingerprint := os.Getenv("STEP_CA_FINGERPRINT")
	
	certPath := os.Getenv("MTLS_CERT_PATH")
	keyPath := os.Getenv("MTLS_KEY_PATH")

	rootCAPath := os.Getenv("STEP_ROOT_CA_PATH")
	if rootCAPath == "" {
		rootCAPath = "/home/step/certs/root_ca.crt"
	}

	dnsName := os.Getenv("DNS_NAME")
	if dnsName == "" {
		dnsName = "localhost"
	}

	return Config{
		Enabled:     enabled,
		Enforced:    enforced,
		CAURL:       caURL,
		Fingerprint: fingerprint,
		CertPath:    certPath,
		KeyPath:     keyPath,
		RootCAPath:  rootCAPath,
		DNSNames:    []string{dnsName},
	}
}

// GetServerConfig returns a *tls.Config for an HTTPS server
func GetServerConfig(ctx context.Context, cfg Config) (*tls.Config, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	rootCAs, err := loadRootCAs(cfg.RootCAPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load root CA: %w", err)
	}

	// For server-side mTLS, we require client certificates
	var clientAuth tls.ClientAuthType = tls.RequireAndVerifyClientCert
	if !cfg.Enforced {
		clientAuth = tls.VerifyClientCertIfGiven
	}

	tlsConfig := &tls.Config{
		ClientCAs:  rootCAs,
		ClientAuth: clientAuth,
		RootCAs:    rootCAs,
	}

	// Try loading from file first (Volume Fallback)
	if cfg.CertPath != "" && cfg.KeyPath != "" {
		cert, err := tls.LoadX509KeyPair(cfg.CertPath, cfg.KeyPath)
		if err == nil {
			tlsConfig.Certificates = []tls.Certificate{cert}
			slog.Info("TLS server configured using volume-mounted certificates", "cert", cfg.CertPath)
			return tlsConfig, nil
		}
		slog.Warn("Failed to load TLS keypair from files, falling back to ACME", "error", err)
	}

	// Use ACME to get a certificate for the server
	manager := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(cfg.DNSNames...),
		Directory:  cfg.CAURL + "/acme/acme/directory",
		Cache:      autocert.DirCache("/tmp/acme-cache"),
		Client: &acme.Client{
			DirectoryURL: cfg.CAURL + "/acme/acme/directory",
			HTTPClient: &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{
						RootCAs: rootCAs,
					},
				},
			},
		},
	}

	tlsConfig.GetCertificate = manager.GetCertificate
	slog.Info("TLS server configured using ACME automation", "ca", cfg.CAURL)
	return tlsConfig, nil
}

// GetClientConfig returns a *tls.Config for an HTTP client calling another mTLS service
func GetClientConfig(ctx context.Context, cfg Config) (*tls.Config, error) {
	if !cfg.Enabled {
		return &tls.Config{InsecureSkipVerify: true}, nil
	}

	rootCAs, err := loadRootCAs(cfg.RootCAPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load root CA: %w", err)
	}

	tlsConfig := &tls.Config{
		RootCAs: rootCAs,
	}

	// Try loading client cert from file first
	if cfg.CertPath != "" && cfg.KeyPath != "" {
		cert, err := tls.LoadX509KeyPair(cfg.CertPath, cfg.KeyPath)
		if err == nil {
			tlsConfig.Certificates = []tls.Certificate{cert}
			slog.Info("TLS client configured using volume-mounted certificates", "cert", cfg.CertPath)
			return tlsConfig, nil
		}
	}

	// Use ACME to get a client certificate
	manager := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Directory:  cfg.CAURL + "/acme/acme/directory",
		Cache:      autocert.DirCache("/tmp/acme-cache-client"),
		HostPolicy: autocert.HostWhitelist(cfg.DNSNames...),
		Client: &acme.Client{
			DirectoryURL: cfg.CAURL + "/acme/acme/directory",
			HTTPClient: &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{
						RootCAs: rootCAs,
					},
				},
			},
		},
	}

	tlsConfig.GetClientCertificate = func(req *tls.CertificateRequestInfo) (*tls.Certificate, error) {
		return manager.GetCertificate(&tls.ClientHelloInfo{
			ServerName: cfg.DNSNames[0],
		})
	}
	slog.Info("TLS client configured using ACME automation", "ca", cfg.CAURL)
	return tlsConfig, nil
}

func loadRootCAs(path string) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	
	// Try loading from file
	pem, err := os.ReadFile(path)
	if err != nil {
		// Fallback: check if we can get it via 'step' CLI if available?
		// No, for now we assume it's mounted.
		return nil, fmt.Errorf("root CA file not found at %s: %w", path, err)
	}

	if ok := pool.AppendCertsFromPEM(pem); !ok {
		return nil, fmt.Errorf("failed to parse root CA certificates from PEM")
	}

	return pool, nil
}

// WatchRotation logs whenever a certificate is renewed (placeholder for now)
func WatchRotation(tlsConfig *tls.Config) {
	go func() {
		for {
			time.Sleep(1 * time.Hour)
			// Log rotation status if we can inspect the manager
			slog.Debug("TLS configuration active", "mTLS", tlsConfig.ClientAuth == tls.RequireAndVerifyClientCert)
		}
	}()
}
