package kafka

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/twmb/franz-go/pkg/kfake"
)

// A test CA and a broker certificate it signed, valid for 127.0.0.1
// (the address the test broker listens on), plus client certificates for mutual
// TLS: one from the test CA and one from an unrelated CA.
type testPKI struct {
	CAPEM    string
	Server   *tls.Config
	OtherPEM string // a CA that signed nothing here

	// A broker that requires a client certificate signed by the test CA.
	MTLSServer         *tls.Config
	ClientCertPEM      string
	ClientKeyPEM       string
	OtherClientCertPEM string // signed by the unrelated CA
	OtherClientKeyPEM  string
}

func newTestPKI(t *testing.T) testPKI {
	t.Helper()
	newCA := func(name string) (*x509.Certificate, *ecdsa.PrivateKey, string) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		tmpl := &x509.Certificate{
			SerialNumber:          big.NewInt(1),
			Subject:               pkix.Name{CommonName: name},
			NotBefore:             time.Now().Add(-time.Hour),
			NotAfter:              time.Now().Add(time.Hour),
			IsCA:                  true,
			BasicConstraintsValid: true,
			KeyUsage:              x509.KeyUsageCertSign,
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
		if err != nil {
			t.Fatal(err)
		}
		cert, _ := x509.ParseCertificate(der)
		return cert, key, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	}

	ca, caKey, caPEM := newCA("kafka_upload test CA")
	otherCA, otherKey, otherPEM := newCA("unrelated CA")

	newClient := func(issuer *x509.Certificate, issuerKey *ecdsa.PrivateKey) (string, string) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		tmpl := &x509.Certificate{
			SerialNumber: big.NewInt(3),
			Subject:      pkix.Name{CommonName: "velociraptor"},
			NotBefore:    time.Now().Add(-time.Hour),
			NotAfter:     time.Now().Add(time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, issuer, &key.PublicKey, issuerKey)
		if err != nil {
			t.Fatal(err)
		}
		keyDER, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			t.Fatal(err)
		}
		return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
			string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
	}
	clientCert, clientKey := newClient(ca, caKey)
	otherClientCert, otherClientKey := newClient(otherCA, otherKey)

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	serverCert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	clientCAs := x509.NewCertPool()
	clientCAs.AddCert(ca)
	return testPKI{
		CAPEM:    caPEM,
		OtherPEM: otherPEM,
		Server: &tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{serverCert},
		},
		MTLSServer: &tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{serverCert},
			ClientAuth:   tls.RequireAndVerifyClientCert,
			ClientCAs:    clientCAs,
		},
		ClientCertPEM:      clientCert,
		ClientKeyPEM:       clientKey,
		OtherClientCertPEM: otherClientCert,
		OtherClientKeyPEM:  otherClientKey,
	}
}

// use_tls=TRUE with root_ca: the right CA delivers, a missing or wrong
// CA is refused with a certificate error, skip_verify accepts any
// certificate, and SCRAM works over TLS.
func TestTLS(t *testing.T) {
	pki := newTestPKI(t)
	c, err := kfake.NewCluster(kfake.NumBrokers(1),
		kfake.SeedTopics(1, "vr.events"),
		kfake.TLS(pki.Server))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	sasl, err := kfake.NewCluster(kfake.NumBrokers(1),
		kfake.SeedTopics(1, "vr.events"),
		kfake.TLS(pki.Server),
		kfake.EnableSASL(),
		kfake.Superuser("SCRAM-SHA-256", "vr", "s3cret"))
	if err != nil {
		t.Fatal(err)
	}
	defer sasl.Close()

	for _, tc := range []struct {
		name         string
		brokers      []string
		extra        string
		sent, failed int
		errorHas     string
	}{
		{"right CA", c.ListenAddrs(), `use_tls=TRUE, root_ca=CA`, 10, 0, ""},
		{"no CA", c.ListenAddrs(), `use_tls=TRUE`, 0, 10, "certificate"},
		{"wrong CA", c.ListenAddrs(), `use_tls=TRUE, root_ca=Other`, 0, 10, "certificate"},
		{"skip_verify", c.ListenAddrs(), `use_tls=TRUE, skip_verify=TRUE`, 10, 0, ""},
		{"SCRAM over TLS", sasl.ListenAddrs(),
			`use_tls=TRUE, root_ca=CA, sasl_mechanism="SCRAM-SHA-256", username="vr", password="s3cret"`, 10, 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scope := newScope(tc.brokers).AppendVars(ordereddict.NewDict().
				Set("CA", pki.CAPEM).Set("Other", pki.OtherPEM))
			rows := runVQL(context.Background(), t, scope, `
SELECT * FROM kafka_upload(
  query={ SELECT * FROM test_rows(count=10) },
  brokers=Brokers, topic="vr.events", timeout=3, `+tc.extra+`)`)
			if len(rows) == 0 {
				t.Fatal("no result rows")
			}
			sent, failed := totals(rows)
			if sent != tc.sent || failed != tc.failed {
				e, _ := rows[0].Get("Error")
				t.Fatalf("sent=%d failed=%d, want %d/%d (error %v)",
					sent, failed, tc.sent, tc.failed, e)
			}
			if tc.errorHas != "" {
				e, _ := rows[0].Get("Error")
				s, _ := e.(string)
				if !strings.Contains(strings.ToLower(s), tc.errorHas) {
					t.Fatalf("error does not mention %q: %q", tc.errorHas, s)
				}
			}
		})
	}
}

// Without use_tls, a TLS listener must not swallow records silently.
func TestPlaintextToTLSListenerFails(t *testing.T) {
	pki := newTestPKI(t)
	c, err := kfake.NewCluster(kfake.NumBrokers(1),
		kfake.SeedTopics(1, "vr.events"), kfake.TLS(pki.Server))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	rows := runVQL(context.Background(), t, newScope(c.ListenAddrs()), `
SELECT * FROM kafka_upload(
  query={ SELECT * FROM test_rows(count=10) },
  brokers=Brokers, topic="vr.events", timeout=2)`)
	if sent, failed := totals(rows); sent != 0 || failed != 10 {
		t.Fatalf("sent=%d failed=%d, want 0/10", sent, failed)
	}
}

// Mutual TLS: a broker that requires a client certificate accepts the
// one signed by its CA and refuses no certificate or one from another CA.
func TestMutualTLS(t *testing.T) {
	pki := newTestPKI(t)
	c, err := kfake.NewCluster(kfake.NumBrokers(1),
		kfake.SeedTopics(1, "vr.events"),
		kfake.TLS(pki.MTLSServer))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	for _, tc := range []struct {
		name         string
		extra        string
		sent, failed int
		errorHas     string
	}{
		{"client cert", `client_cert=Cert, client_key=Key`, 10, 0, ""},
		{"no client cert", ``, 0, 10, "certificate"},
		{"client cert from another CA", `client_cert=OtherCert, client_key=OtherKey`, 0, 10, "certificate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			extra := ""
			if tc.extra != "" {
				extra = ", " + tc.extra
			}
			scope := newScope(c.ListenAddrs()).AppendVars(ordereddict.NewDict().
				Set("CA", pki.CAPEM).
				Set("Cert", pki.ClientCertPEM).Set("Key", pki.ClientKeyPEM).
				Set("OtherCert", pki.OtherClientCertPEM).Set("OtherKey", pki.OtherClientKeyPEM))
			rows := runVQL(context.Background(), t, scope, `
SELECT * FROM kafka_upload(
  query={ SELECT * FROM test_rows(count=10) },
  brokers=Brokers, topic="vr.events", timeout=3,
  use_tls=TRUE, root_ca=CA`+extra+`)`)
			if len(rows) == 0 {
				t.Fatal("no result rows")
			}
			sent, failed := totals(rows)
			e, _ := rows[0].Get("Error")
			if sent != tc.sent || failed != tc.failed {
				t.Fatalf("sent=%d failed=%d, want %d/%d (error %v)",
					sent, failed, tc.sent, tc.failed, e)
			}
			if tc.errorHas != "" {
				s, _ := e.(string)
				if !strings.Contains(strings.ToLower(s), tc.errorHas) {
					t.Fatalf("error does not mention %q: %q", tc.errorHas, s)
				}
			}
		})
	}
}

// Client certificate arguments that cannot work are refused up front,
// with a message saying why, before anything is sent.
func TestClientCertArgErrors(t *testing.T) {
	pki := newTestPKI(t)
	for _, tc := range []struct {
		name, extra, logHas string
	}{
		{"cert without key", `use_tls=TRUE, client_cert=Cert`, "must be given together"},
		{"key without cert", `use_tls=TRUE, client_key=Key`, "must be given together"},
		{"without use_tls", `client_cert=Cert, client_key=Key`, "needs use_tls"},
		{"key does not match", `use_tls=TRUE, client_cert=Cert, client_key=OtherKey`, "unable to load client_cert"},
		{"not PEM", `use_tls=TRUE, client_cert="nonsense", client_key=Key`, "unable to load client_cert"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs := &logBuffer{}
			scope := newScope([]string{"127.0.0.1:1"}).AppendVars(ordereddict.NewDict().
				Set("Cert", pki.ClientCertPEM).Set("Key", pki.ClientKeyPEM).
				Set("OtherKey", pki.OtherClientKeyPEM))
			scope.SetLogger(log.New(logs, "", 0))
			rows := runVQL(context.Background(), t, scope, `
SELECT * FROM kafka_upload(
  query={ SELECT * FROM test_rows(count=1) },
  brokers=Brokers, topic="vr.events", timeout=1, `+tc.extra+`)`)
			if len(rows) != 0 {
				t.Fatalf("got %d result rows, want none", len(rows))
			}
			if !strings.Contains(logs.String(), tc.logHas) {
				t.Fatalf("log does not mention %q: %q", tc.logHas, logs.String())
			}
			// The key must never appear in the log.
			if strings.Contains(logs.String(), "PRIVATE KEY") {
				t.Fatal("private key leaked into the log")
			}
		})
	}
}

// The secret can carry the client certificate and key.
func TestApplySecretClientCert(t *testing.T) {
	arg := &KafkaUploadArgs{Secret: "s"}
	err := ApplySecret(arg, ordereddict.NewDict().
		Set("brokers", "b:9093").
		Set("client_cert", "-----BEGIN CERTIFICATE-----\nx\n-----END CERTIFICATE-----\n").
		Set("client_key", "  -----BEGIN PRIVATE KEY-----\ny\n-----END PRIVATE KEY-----  "))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(arg.ClientCert, "-----BEGIN CERTIFICATE-----") ||
		!strings.HasPrefix(arg.ClientKey, "-----BEGIN PRIVATE KEY-----") {
		t.Fatalf("client_cert=%q client_key=%q", arg.ClientCert, arg.ClientKey)
	}
}
