package provider

import (
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"golang.org/x/crypto/ssh"
	"software.sslmate.com/src/go-pkcs12"
)

func decodePEM(d *schema.ResourceData, pemKey, pemType string) (*pem.Block, error) {
	block, _ := pem.Decode([]byte(d.Get(pemKey).(string)))
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in %s", pemKey)
	}
	if pemType != "" && block.Type != pemType {
		return nil, fmt.Errorf("invalid PEM type in %s: %s", pemKey, block.Type)
	}

	return block, nil
}

func parsePrivateKey(d *schema.ResourceData, pemKey, algoKey string) (interface{}, error) {
	algoName := d.Get(algoKey).(string)

	keyFunc, ok := keyParsers[algoName]
	if !ok {
		return nil, fmt.Errorf("invalid %s: %#v", algoKey, algoName)
	}

	block, err := decodePEM(d, pemKey, "")
	if err != nil {
		return nil, err
	}

	key, err := keyFunc(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to decode %s: %s", pemKey, err)
	}

	return key, nil
}

func parseCertificate(d *schema.ResourceData, pemKey string) (*x509.Certificate, error) {
	block, err := decodePEM(d, pemKey, "")
	if err != nil {
		return nil, err
	}

	certs, err := x509.ParseCertificates(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %s", pemKey, err)
	}
	if len(certs) < 1 {
		return nil, fmt.Errorf("no certificates found in %s", pemKey)
	}
	if len(certs) > 1 {
		return nil, fmt.Errorf("multiple certificates found in %s", pemKey)
	}

	return certs[0], nil
}

func parseCertificateRequest(d *schema.ResourceData, pemKey string) (*x509.CertificateRequest, error) {
	block, err := decodePEM(d, pemKey, pemCertReqType)
	if err != nil {
		return nil, err
	}

	certReq, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %s", pemKey, err)
	}

	return certReq, nil
}

func readPublicKey(d *schema.ResourceData, rsaKey interface{}) error {
	pubKey := publicKey(rsaKey)
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(pubKey)
	if err != nil {
		return fmt.Errorf("failed to marshal public key error: %s", err)
	}
	pubKeyPemBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKeyBytes,
	}

	d.SetId(hashForState(string((pubKeyBytes))))
	d.Set("public_key_pem", string(pem.EncodeToMemory(pubKeyPemBlock)))

	sshPubKey, err := ssh.NewPublicKey(publicKey(rsaKey))
	if err == nil {
		// Not all EC types can be SSH keys, so we'll produce this only
		// if an appropriate type was selected.
		sshPubKeyBytes := ssh.MarshalAuthorizedKey(sshPubKey)
		d.Set("public_key_openssh", string(sshPubKeyBytes))
		d.Set("public_key_fingerprint_md5", ssh.FingerprintLegacyMD5(sshPubKey))
	} else {
		d.Set("public_key_openssh", "")
		d.Set("public_key_fingerprint_md5", "")
	}
	return nil
}

func createB64Pfx(privateKey interface{}, certificate *x509.Certificate, caCerts []*x509.Certificate, password string) ([]byte, error) {
	pfxData, err := pkcs12.Encode(rand.Reader, privateKey, certificate, caCerts, password)
	if err != nil {
		return nil, err
	}
	b64Buffer := make([]byte, base64.StdEncoding.EncodedLen(len(pfxData)))
	base64.StdEncoding.Encode(b64Buffer, pfxData)
	return b64Buffer, nil
}
