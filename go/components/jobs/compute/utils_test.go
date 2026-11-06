package compute

import (
	"testing"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/secrets"
	"github.com/stretchr/testify/require"
)

func TestGetKubeClientConfigFromConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		auth      secrets.ClientAuth
		wantToken string
		wantCert  []byte
		wantKey   []byte
		wantErr   string
	}{
		{
			name: "bearer token",
			auth: secrets.ClientAuth{
				CertificateAuthorityData: "ca-data",
				ClientTokenData:          "token-data",
			},
			wantToken: "token-data",
		},
		{
			name: "client certificate",
			auth: secrets.ClientAuth{
				CertificateAuthorityData: "ca-data",
				ClientCertificateData:    "cert-data",
				ClientKeyData:            "key-data",
			},
			wantCert: []byte("cert-data"),
			wantKey:  []byte("key-data"),
		},
		{
			name: "client certificate and bearer token",
			auth: secrets.ClientAuth{
				CertificateAuthorityData: "ca-data",
				ClientCertificateData:    "cert-data",
				ClientKeyData:            "key-data",
				ClientTokenData:          "token-data",
			},
			wantToken: "token-data",
			wantCert:  []byte("cert-data"),
			wantKey:   []byte("key-data"),
		},
		{
			name: "client certificate without key",
			auth: secrets.ClientAuth{
				CertificateAuthorityData: "ca-data",
				ClientCertificateData:    "cert-data",
			},
			wantErr: "client-key-data or client-key must be specified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := GetKubeClientConfigFromConfiguration(
				"test-service", "https://example.com:443", "test-namespace", &tt.auth, false)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "https://example.com:443", cfg.Host)
			require.Equal(t, []byte("ca-data"), cfg.TLSClientConfig.CAData)
			require.Equal(t, tt.wantToken, cfg.BearerToken)
			require.Equal(t, tt.wantCert, cfg.TLSClientConfig.CertData)
			require.Equal(t, tt.wantKey, cfg.TLSClientConfig.KeyData)
		})
	}
}
