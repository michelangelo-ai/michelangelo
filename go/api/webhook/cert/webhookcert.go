package webhookcert

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"time"
)

const (
	CertDir    = "/tmp/k8s-webhook-server/serving-certs"
	ServerCert = CertDir + "/tls.crt"
	ServerKey  = CertDir + "/tls.key"
	CACert     = CertDir + "/ca.crt"
)

// EnsureWebhookCertOnDisk creates or reuses certs for the webhook server.
// Params:
//
//	host (string): The host name for the webhook server.
//
// Returns:
//
//	error: Error if the certificate creation or writing fails.
func EnsureWebhookCertOnDisk(host string) error {
	if _, err := os.Stat(ServerCert); err == nil {
		// already exists
		return nil
	}
	if err := os.MkdirAll(CertDir, 0700); err != nil {
		return err
	}

	// Generate CA
	caKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	ca := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "webhook-ca"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	caDER, _ := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)

	// Generate server cert
	serverKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	serverCert := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: host},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		DNSNames:     []string{host},
	}
	serverDER, _ := x509.CreateCertificate(rand.Reader, serverCert, ca, &serverKey.PublicKey, caKey)

	// Write PEMs
	write := func(path, typ string, b []byte) error {
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		defer f.Close()
		return pem.Encode(f, &pem.Block{Type: typ, Bytes: b})
	}

	if err := write(ServerCert, "CERTIFICATE", serverDER); err != nil {
		return err
	}
	if err := write(ServerKey, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(serverKey)); err != nil {
		return err
	}
	if err := write(CACert, "CERTIFICATE", caDER); err != nil {
		return err
	}
	return nil
}
