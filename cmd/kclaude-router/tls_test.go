package main

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCertificateIsPrivatePersistentAndVerifiesLoopback(t *testing.T) {
	state := t.TempDir()
	certificate, err := localCertificate(state)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := localCertificate(state)
	if err != nil {
		t.Fatal(err)
	}
	if string(certificate.Certificate[0]) != string(loaded.Certificate[0]) {
		t.Fatal("certificate changed on restart")
	}
	info, err := os.Stat(filepath.Join(state, "router-tls.key"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("private TLS key permissions", err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}}
	srv.StartTLS()
	defer srv.Close()
	roots := x509.NewCertPool()
	pem, err := os.ReadFile(filepath.Join(state, "router-cert.pem"))
	if err != nil || !roots.AppendCertsFromPEM(pem) {
		t.Fatal("invalid certificate")
	}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots}}}
	response, err := client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	// A different profile's valid certificate cannot impersonate this router.
	other, err := localCertificate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	otherRoots := x509.NewCertPool()
	parsed, err := x509.ParseCertificate(other.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	otherRoots.AddCert(parsed)
	untrusted := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: otherRoots}}}
	if resp, err := untrusted.Get(srv.URL); err == nil {
		resp.Body.Close()
		t.Fatal("untrusted router certificate accepted")
	}
}
