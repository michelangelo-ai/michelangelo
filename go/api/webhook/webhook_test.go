package webhook

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/config"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// newTestParams creates a new Params for testing.
func newTestParams(t *testing.T, cfg *Configuration) Params {
	return Params{
		Config:  cfg,
		Scheme:  runtime.NewScheme(),
		Logger:  zaptest.NewLogger(t),
		Metrics: tally.NoopScope,
	}
}

// createDummyCerts creates dummy tls.crt, tls.key and ca.crt files in the given directory.
func createDummyCerts(t *testing.T, dir string) {
	caCrt := `-----BEGIN CERTIFICATE-----
MIIDCDCCAfCgAwIBAgITEST/ERQ+mSdSKQk9f2NzlbfgFjANBgkqhkiG9w0BAQsF
ADATMREwDwYDVQQDDAhNeVRlc3RDQTAgFw0yNTA2MDMyMjI2MzRaGA8yMDUyMTAx
OTIyMjYzNFowEzERMA8GA1UEAwwITXlUZXN0Q0EwggEiMA0GCSqGSIb3DQEBAQUA
A4IBDwAwggEKAoIBAQC6ZhK0t++pFV2lSyLl5pFixf0kGHSr2mKfuIFkTYeAKisj
feiCWTXvIIEN1lO6osN6r5+pMv6LE+TCHfUsXPZe762JZ37og9JdnzzhbWSg4YEP
EsqsRmxccXgq/Q0ESXL7Duq710sRgrMxgaMYLe5Yz50sTJgfKjBvoaLGMz6Y9SPP
Qm/a9zn+kdP8j6/SASXa/zI6RZSRho8BiPNmhXkEj3Mt4nFq06p941vJU7wRmEYX
ttpr1azuBJvbsloRMdIUaKLonhLhfJSFxM6i61kKdwYLKeuk1oR+vBLybHBEFxzR
kneG1ATjNTWMHFCFQvnklbhrF2SWS9cxDSOx6FpTAgMBAAGjUzBRMB0GA1UdDgQW
BBSumYUuuDhklehVl0JvnvaOoJm09TAfBgNVHSMEGDAWgBSumYUuuDhklehVl0Jv
nvaOoJm09TAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQAUnN+x
26ijD+IWfCpi8tWWtez1v1XOaiN0P16AQPWw6EmBJhFPGM1sEeW1GFC5eABDygid
yIREzIXX/XmOwuxouBI8oComm3X9o9XWhkMTyWS2MBUvMh8V9v4h61aRxGMfI3rK
am+T0FEhVyVxqYXffM66i9ux/1rm4++4GaN0TTtGJySa3rw3yj3CjZODfLUHKhOi
LYP19cHi0FsRwY1Dw7H5xEdyX0/HNrgVZFxFfMJts4MXHGiEOv0Vh/90TqgbS3YY
vxzdIpe1bp/+gpjVntM0kT46divBczhqBDnDDsYqgrw9LD1VhjP5XaTn//JcLIfP
CrjDtZGb6Hpui7Ph
-----END CERTIFICATE-----`
	tlsCrt := `-----BEGIN CERTIFICATE-----
MIIDQTCCAimgAwIBAgIUU7Zg1VC/flid2JtPa+82olDRccswDQYJKoZIhvcNAQEL
BQAwEzERMA8GA1UEAwwITXlUZXN0Q0EwHhcNMjUwNjAzMjIyODMxWhcNMjYwNjAz
MjIyODMxWjAeMRwwGgYDVQQDDBN0ZXN0LXdlYmhvb2stc2VydmVyMIIBIjANBgkq
hkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAqzEvDzCkDyi1KUIDBu5uDKKKfkooof3M
6gU6F0ss0AFIUMGC5FpqmSwTNk7JxtRak6QtuSvkD9LZfNtErb/5YuFqWRkSRgZz
ROYsW/WjycWk//pBbWHm7vjkACw9yjCj1CwaoO/Vdk1PwrUT4Nap1iZFLUS+jpkl
Nw3R4IhY8v8WRbbg1zN7SnEcoHgHu98X3tSnQhqI0V8drOvhMMYnr3NkA0vUtn6A
EkA+kVxrIMsKa33JbACkcs+Guc7M3NIYKOmuNvPbatwtW0hBJEGg/DXZq9jsFpDP
skoItJH1j0hmr6rmgFw1YS7qoT79+PSNnrUkbAsc6H3/O87ov+aIUwIDAQABo4GB
MH8wHwYDVR0jBBgwFoAUrpmFLrg4ZJXoVZdCb572jqCZtPUwCQYDVR0TBAIwADAL
BgNVHQ8EBAMCBPAwJQYDVR0RBB4wHIcEfwAAAYIUaG9zdC5kb2NrZXIuaW50ZXJu
YWwwHQYDVR0OBBYEFEfhETx821ftEu+og30eYYFFLyWZMA0GCSqGSIb3DQEBCwUA
A4IBAQBqQ6sz2/Q27IBmLcn8rbT5DDOwAX6b7q+QDB6ggfeqjMdchbkPj0X7ICJp
8gmtkDQzfkzr7Nh7OPs0pIZLM4DV+BQn5lq6Y6K+QsHfBr1EqI+VtfEnRGW6b0U9
H4Ey/XAD/X+n2w0zIgIfVmlw2zmQUzEUX/isIBNK4eWAyUAYIMh8To5Zr2ewGa8L
PCeV4Zh50dno1D7dn4PD3BUj9IXoF1DmGGl7q3/ZcBw51cB3u9UAo0hss1p7Mkai
tsXlsxefmwRqC7/StEW1Mdm8pQnZdNjJtLy+ko7i7LztRgQ3t7489KhQQ7l/vJr9
tuTntNMpHDZ5Yo3rGh12hW3GBTek
-----END CERTIFICATE-----`
	tlsKey := `-----BEGIN PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQCrMS8PMKQPKLUp
QgMG7m4Moop+Siih/czqBToXSyzQAUhQwYLkWmqZLBM2TsnG1FqTpC25K+QP0tl8
20Stv/li4WpZGRJGBnNE5ixb9aPJxaT/+kFtYebu+OQALD3KMKPULBqg79V2TU/C
tRPg1qnWJkUtRL6OmSU3DdHgiFjy/xZFtuDXM3tKcRygeAe73xfe1KdCGojRXx2s
6+Ewxievc2QDS9S2foASQD6RXGsgywprfclsAKRyz4a5zszc0hgo6a4289tq3C1b
SEEkQaD8Ndmr2OwWkM+ySgi0kfWPSGavquaAXDVhLuqhPv349I2etSRsCxzoff87
zui/5ohTAgMBAAECggEAQFBI+q7uY5eKf8aB9p+qjmqeFxXrL/h2fFCcY1Xlrvtc
XKJmdz2UoJjTWuq8mUr8AE2Es/VOR7eR53tE0PW3TjObTX/CwrX3piHG9oFRGCN9
eoFdBSfrp0mv9nSofgZJ9hLfqiiQDFK9LUvz/NsIkSBtirUx1capGYbCm1T9/cPC
vr6KsbzbQF3g1SIa402HwhguZGoELveVo69wUM4RBet1MpNukhXYNjbJYkQA2yvR
wSB4wIxZ5lliY5mI/aTFiNXr34bQG9i/yvwe24LCy+hXNCmkuyi/1DSKxRyBrH02
QDbUZftQ/Vt6vQ0tFa3RPpckJn04fOvwmVabnhCcaQKBgQDvCiQB00J8wMGXrL5L
Jc4MIsSapUuMrHY04xjQw88Kg4TyOfQ200t9YXW8m8FpZ9FfBk5xERT6IZ/pdXqA
wbvfDlh3rsPfpoi4E49kRzfVGkrnpROHS4L+Ts8ujFPFh0TyKtwS9eBrptydVqKH
wHK/UcMCZw9Id2Om+P1upepWLwKBgQC3Vq1mvj2IBfo+ufJSAZra+UDcbyNNsWUC
VSGOS/rj3CqNTFsxJ3STKj7pzMpwf0BCgSmgsPW5vWwe75bjURA4xm9vJHTnGZaJ
dvhHjqzhSMUI1WebtIHFu7spaVmZFseBUFc0pXfmSyKAxmVVCHNCp/vDh5hIf6e3
RYpQ6NVLHQKBgDCcz1XPsOXODZDbAJgnyA+PwovwsbyaFjALPzC1oZVxyce5IYFE
10VYXKlOw7a79khs7+buomV8ERlZWuB0hdCHClbMo+kH5SYKVE8AbMpZ3oHdgGsz
YCB3xoqg3yh8qfjV3ou8lTdPZ+5XgBY7fRqLdi026FTEcu+yE1g9RbrhAoGBAJ3C
3k+M4FHOIvoa8+ORMfm/hgqpL83JGkwZiVhzFR9B8vPHgqkXdH62WZDCAmkvdtJD
Zti5rZj44LL2I/bTaIwSZQ1UZ6v9HsaHMzoQEb+B6NqjGBaqCwllc7Y8yzaqnV4v
Dftlb3khqjz5e3TiYpw3BLPKWEX6Yw2Xr1/UGsYZAoGBANuR4V3UwI290wGbX3dz
v0k4fa9EZAyhegV99m68zFUQZbDs4b4RqSCQwJF545XLmGF87cStZB7l3YP78Q1l
LQYjVq6m7e4yEpVr37dJByfJTjKDVdtr/YI0ox0wO3HTNNH/6wmUC/YzOuKYxqbo
j0fH1J48u+CNex94xRG5GWZ2
-----END PRIVATE KEY-----`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ca.crt"), []byte(caCrt), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tls.crt"), []byte(tlsCrt), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tls.key"), []byte(tlsKey), 0644))
}

func TestStartWebhookServer_Success(t *testing.T) {
	// Create a temporary directory for the certificates.
	tempDir := t.TempDir()
	createDummyCerts(t, tempDir)

	// Use port 0 to let the OS pick a free ephemeral port.
	cfg := &Configuration{Host: "127.0.0.1", Port: 0, CertDir: tempDir}
	params := newTestParams(t, cfg)

	err, cancel := startWebhookServer(params)
	require.NoError(t, err)
	require.NotNil(t, cancel)

	// Clean up the running server.
	defer cancel()
}

func TestStartWebhookServer_PortInUse(t *testing.T) {
	// Occupy a port first.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	// Get the port that the listener is using.
	port := listener.Addr().(*net.TCPAddr).Port

	cfg := &Configuration{Host: "127.0.0.1", Port: port, CertDir: t.TempDir()}
	createDummyCerts(t, cfg.CertDir) // Create dummy certs even for failure case
	params := newTestParams(t, cfg)

	err, cancel := startWebhookServer(params)
	require.Error(t, err)
	// cancel should be nil on failure.
	assert.Nil(t, cancel)
	assert.Contains(t, err.Error(), "webhook server failed to start")
}

func TestParseConfig_PopulateError(t *testing.T) {
	t.Parallel()

	// 'port' is a string, but the struct expects an int, causing a populate error.
	yamlConfig := `
webhook:
  host: "my-host"
  port: "not-an-int"
  certDir: "/my/cert/dir"
  url: "https://my.webhook.url"
`
	provider, err := config.NewYAML(config.Source(strings.NewReader(yamlConfig)))
	require.NoError(t, err)

	_, err = parseConfig(provider)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot unmarshal")
}

func TestWebhookModule_Lifecycle(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	createDummyCerts(t, tempDir)

	yamlConfig := `
webhook:
  host: "127.0.0.1"
  port: 0
  certDir: "` + tempDir + `"
  url: "https://my.webhook.url"
`
	provider, err := config.NewYAML(config.Source(strings.NewReader(yamlConfig)))
	require.NoError(t, err)

	testApp := fxtest.New(
		t,
		fx.Provide(
			func() config.Provider { return provider },
			func() *runtime.Scheme { return runtime.NewScheme() },
			func() *zap.Logger { return zaptest.NewLogger(t) },
			func() tally.Scope { return tally.NoopScope },
		),
		Module,
		fx.Invoke(func(clientConfig *apiextv1.WebhookClientConfig) {
			require.NotNil(t, clientConfig)
			assert.Equal(t, "https://my.webhook.url/convert", *clientConfig.URL)
			require.NotEmpty(t, clientConfig.CABundle)
		}),
	)

	testApp.RequireStart()
	testApp.RequireStop()
}

func TestGetWebhookClientConfig_FileReadError(t *testing.T) {
	t.Parallel()

	emptyDir := t.TempDir()

	params := newTestParams(t, &Configuration{
		CertDir: emptyDir,
		URL:     "https://my-test-url.com",
	})

	_, err := getWebhookClientConfig(params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read CA certificate")
}
