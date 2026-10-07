package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// Each private state directory gets a persistent, locally trusted certificate.
// It authenticates every Claude connection, including after a router restart.
func localCertificate(state string) (tls.Certificate, error) {
	certPath := filepath.Join(state, "router-cert.pem")
	keyPath := filepath.Join(state, "router-tls.key")
	if _, err := os.Stat(certPath); err == nil {
		return tls.LoadX509KeyPair(certPath, keyPath)
	} else if !os.IsNotExist(err) {
		return tls.Certificate{}, err
	}
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}
	template := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: "kclaude local router"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(5, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true, IsCA: true,
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, DNSNames: []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return tls.Certificate{}, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	// The launcher holds start.lock while spawning this process. Write the cert
	// last: if startup is interrupted, an absent cert regenerates the pair.
	for _, file := range []struct {
		path string
		data []byte
	}{{keyPath, keyPEM}, {certPath, certPEM}} {
		temporary, err := os.CreateTemp(state, ".tls-")
		if err != nil {
			return tls.Certificate{}, err
		}
		name := temporary.Name()
		_, writeErr := temporary.Write(file.data)
		closeErr := temporary.Close()
		if writeErr != nil || closeErr != nil {
			os.Remove(name)
			return tls.Certificate{}, fmt.Errorf("write TLS material: %v %v", writeErr, closeErr)
		}
		if err := os.Rename(name, file.path); err != nil {
			os.Remove(name)
			return tls.Certificate{}, err
		}
	}
	return tls.X509KeyPair(certPEM, keyPEM)
}
