package cert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type TailscaleStatus struct {
	Self struct {
		DNSName      string   `json:"DNSName"`
		TailscaleIPs []string `json:"TailscaleIPs"`
	} `json:"Self"`
}

// GetTailscaleInfo queries Tailscale status to find the node's domain name and IP addresses
func GetTailscaleInfo() (domain string, ips []net.IP, err error) {
	cmd := exec.Command("tailscale", "status", "--json")
	out, err := cmd.Output()
	if err != nil {
		return "", nil, err
	}

	var status TailscaleStatus
	if err := json.Unmarshal(out, &status); err != nil {
		return "", nil, err
	}

	domain = strings.TrimSuffix(status.Self.DNSName, ".")
	for _, ipStr := range status.Self.TailscaleIPs {
		if ip := net.ParseIP(ipStr); ip != nil {
			ips = append(ips, ip)
		}
	}
	return domain, ips, nil
}

// DiscoverTailscaleCerts checks if a Tailscale Let's Encrypt certificate exists for this node
func DiscoverTailscaleCerts(domain string) (certPath, keyPath string, found bool) {
	if domain == "" {
		return "", "", false
	}

	// Tailscale writes files named [domain].crt and [domain].key
	certName := domain + ".crt"
	keyName := domain + ".key"

	home, err := os.UserHomeDir()
	searchPaths := []string{"."}
	if err == nil {
		searchPaths = append(searchPaths, home, filepath.Join(home, ".1agents", "certs"))
	}

	for _, dir := range searchPaths {
		c := filepath.Join(dir, certName)
		k := filepath.Join(dir, keyName)
		if _, errC := os.Stat(c); errC == nil {
			if _, errK := os.Stat(k); errK == nil {
				return c, k, true
			}
		}
	}
	return "", "", false
}

// GenerateSelfSignedCert creates a self-signed X.509 certificate and private key
// and writes them to the specified files in PEM format.
func GenerateSelfSignedCert(certPath, keyPath string, tsDomain string, tsIPs []net.IP) error {
	dir := filepath.Dir(certPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory for cert: %w", err)
	}
	dirKey := filepath.Dir(keyPath)
	if err := os.MkdirAll(dirKey, 0755); err != nil {
		return fmt.Errorf("failed to create directory for key: %w", err)
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate private key: %w", err)
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(3650 * 24 * time.Hour) // 10 years

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return fmt.Errorf("failed to generate serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"RemoteAgent"},
			CommonName:   "RemoteAgent Self-Signed",
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	template.DNSNames = []string{"localhost"}
	template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1"), net.IPv6loopback}

	// Add local system network IPs
	addrs, err := net.InterfaceAddrs()
	if err == nil {
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && !ip.IsLoopback() {
				template.IPAddresses = append(template.IPAddresses, ip)
			}
		}
	}

	// Append Tailscale specific identifiers
	if tsDomain != "" {
		template.DNSNames = append(template.DNSNames, tsDomain)
	}
	template.IPAddresses = append(template.IPAddresses, tsIPs...)

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return fmt.Errorf("failed to create certificate: %w", err)
	}

	certOut, err := os.Create(certPath)
	if err != nil {
		return fmt.Errorf("failed to open cert file for writing: %w", err)
	}
	defer certOut.Close()

	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return fmt.Errorf("failed to encode certificate: %w", err)
	}

	keyOut, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open key file for writing: %w", err)
	}
	defer keyOut.Close()

	privBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return fmt.Errorf("failed to marshal private key: %w", err)
	}

	if err := pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes}); err != nil {
		return fmt.Errorf("failed to encode private key: %w", err)
	}

	return nil
}
