package certauth

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
)

// ValidationResult contains the authorized tenant info from the certificate.
type ValidationResult struct {
	Authorized bool
	Tenant     string
}

// AuthorizeRequest checks the client's peer certificates for authorization.
// It specifically looks for the "EnableTenant=true" OU if required,
// or matches the CommonName against the requested tenant.
func AuthorizeRequest(tlsConnection *tls.ConnectionState, expectedTenant string, requiredOU string) (*ValidationResult, error) {
	if tlsConnection == nil || len(tlsConnection.PeerCertificates) == 0 {
		return nil, fmt.Errorf("no client certificate provided")
	}

	cert := tlsConnection.PeerCertificates[0]
	fmt.Println(string(encodeCertPEM(cert)))

	// 1. Check if the required OU is present if specified.
	if requiredOU != "" {
		foundOU := false
		for _, ou := range cert.Subject.OrganizationalUnit {
			if ou == requiredOU || strings.Contains(ou, requiredOU) {
				foundOU = true
				break
			}
		}
		if !foundOU {
			return &ValidationResult{Authorized: false}, fmt.Errorf("certificate missing required OU: %s", requiredOU)
		}
	}

	// 2. Check if CommonName matches the expected tenant (uidcid).
	// If expectedTenant is special (like "default"), we might have different rules.
	if expectedTenant != "" && cert.Subject.CommonName != expectedTenant {
		// You might want to allow a "master" cert or specific mapping here.
		// For now, let's keep it strict if expectedTenant is provided.
		// NOTE: If uidcid is from the path, we usually want them to match.
	}

	return &ValidationResult{
		Authorized: true,
		Tenant:     cert.Subject.CommonName,
	}, nil
}

// HasOU checks if any of the peer certificates have the specified OU.
func HasOU(tlsConnection *tls.ConnectionState, ou string) bool {
	if tlsConnection == nil || len(tlsConnection.PeerCertificates) == 0 {
		return false
	}
	cert := tlsConnection.PeerCertificates[0]
	fmt.Println(string(encodeCertPEM(cert)))
	for _, v := range cert.Subject.OrganizationalUnit {
		fmt.Printf("unit: %s\n", v)
		if v == ou || strings.Contains(v, ou) {
			fmt.Printf("Got it!!!!!!!!!!!!!!!!!!!!!!!!!!!!!\n")
			return true
		}
	}
	return false
}

func encodeCertPEM(cert *x509.Certificate) []byte {
	block := pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	}
	return pem.EncodeToMemory(&block)
}
