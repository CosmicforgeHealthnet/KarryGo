package bridge

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func workloadCertificate(t *testing.T, uri string, expired bool) (tls.Certificate, *x509.Certificate, *x509.CertPool) {
	t.Helper()
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	now := time.Now()
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Disposable test workload CA"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err = x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	uriValue, _ := url.Parse(uri)
	clientPub, clientKey, _ := ed25519.GenerateKey(rand.Reader)
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, KeyUsage: x509.KeyUsageDigitalSignature, URIs: []*url.URL{uriValue}}
	if expired {
		leaf.NotAfter = now.Add(-time.Minute)
	}
	der, err := x509.CreateCertificate(rand.Reader, leaf, ca, clientPub, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(clientKey)
	if err != nil {
		t.Fatal(err)
	}
	pair, err := tls.X509KeyPair(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	return pair, leaf, roots
}

func TestRealMTLSBoundary(t *testing.T) {
	for _, scenario := range []string{"valid", "missing", "untrusted", "expired", "wrong_principal"} {
		t.Run(scenario, func(t *testing.T) {
			f := newFixture(t)
			uri := f.b.trust.WorkloadURI
			if scenario == "wrong_principal" {
				uri = "spiffe://test/healthnet/hq"
			}
			pair, leaf, roots := workloadCertificate(t, uri, scenario == "expired")
			f.cert = leaf
			srv := httptest.NewUnstartedServer(f.b)
			srv.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}
			srv.StartTLS()
			defer srv.Close()
			client := srv.Client()
			transport := client.Transport.(*http.Transport)
			transport.TLSClientConfig.Certificates = []tls.Certificate{pair}
			if scenario == "missing" {
				transport.TLSClientConfig.Certificates = nil
			}
			if scenario == "untrusted" {
				other, _, _ := workloadCertificate(t, f.b.trust.WorkloadURI, false)
				transport.TLSClientConfig.Certificates = []tls.Certificate{other}
			}
			r := f.request(t, "GET", "/capabilities"+scopeQuery, nil)
			target, _ := url.Parse(srv.URL)
			r.URL.Scheme = target.Scheme
			r.URL.Host = target.Host
			r.RequestURI = ""
			res, err := client.Do(r)
			switch scenario {
			case "valid":
				if err != nil {
					t.Fatal(err)
				}
				defer res.Body.Close()
				if res.StatusCode != 200 {
					t.Fatal("valid mTLS failed", res.StatusCode)
				}
			case "wrong_principal":
				if err != nil {
					t.Fatal(err)
				}
				defer res.Body.Close()
				if res.StatusCode != 401 {
					t.Fatal("other workload admitted", res.StatusCode)
				}
			default:
				if err == nil {
					res.Body.Close()
					t.Fatal("invalid certificate completed TLS handshake")
				}
			}
			if f.repo.records != 0 {
				t.Fatal("TLS denial caused an operation")
			}
		})
	}
}
