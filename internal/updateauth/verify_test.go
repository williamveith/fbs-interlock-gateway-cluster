package updateauth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyUpdateChecksumCommandValidSignature(t *testing.T) {

	publicKey, privateKey := newTestSigningKey(t)

	const (
		asset    = "fbs-interlock-gateway-cluster-linux-amd64"
		checksum = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	)

	checksumData := []byte(checksum + "  " + asset + "\n")
	signature := ed25519.Sign(privateKey, checksumData)

	withTestPublicKey(t, publicKey)

	checksumPath := writeTestFile(t, "asset.sha256", checksumData)
	signaturePath := writeTestFile(t, "asset.sha256.sig", signature)

	got, err := VerifyUpdateChecksumCommand([]string{
		"--checksum", checksumPath,
		"--signature", signaturePath,
		"--asset", asset,
	})
	if err != nil {
		t.Fatalf("VerifyUpdateChecksumCommand() error = %v", err)
	}

	if got != checksum {
		t.Fatalf("VerifyUpdateChecksumCommand() = %q, want %q", got, checksum)
	}
}

func TestVerifyUpdateChecksumCommandRejectsModifiedChecksum(t *testing.T) {

	publicKey, privateKey := newTestSigningKey(t)

	const asset = "fbs-interlock-gateway-cluster-linux-amd64"

	original := []byte(
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef  " +
			asset + "\n",
	)
	signature := ed25519.Sign(privateKey, original)

	modified := append([]byte(nil), original...)
	modified[0] = 'f'

	withTestPublicKey(t, publicKey)

	checksumPath := writeTestFile(t, "asset.sha256", modified)
	signaturePath := writeTestFile(t, "asset.sha256.sig", signature)

	_, err := VerifyUpdateChecksumCommand([]string{
		"--checksum", checksumPath,
		"--signature", signaturePath,
		"--asset", asset,
	})
	if err == nil {
		t.Fatal("VerifyUpdateChecksumCommand() succeeded with a modified checksum file")
	}

	if !strings.Contains(err.Error(), "invalid update checksum signature") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyUpdateChecksumCommandRejectsModifiedSignature(t *testing.T) {

	publicKey, privateKey := newTestSigningKey(t)

	const asset = "fbs-interlock-gateway-cluster-linux-amd64"

	checksumData := []byte(
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef  " +
			asset + "\n",
	)
	signature := ed25519.Sign(privateKey, checksumData)
	signature[0] ^= 0xff

	withTestPublicKey(t, publicKey)

	checksumPath := writeTestFile(t, "asset.sha256", checksumData)
	signaturePath := writeTestFile(t, "asset.sha256.sig", signature)

	_, err := VerifyUpdateChecksumCommand([]string{
		"--checksum", checksumPath,
		"--signature", signaturePath,
		"--asset", asset,
	})
	if err == nil {
		t.Fatal("VerifyUpdateChecksumCommand() succeeded with a modified signature")
	}

	if !strings.Contains(err.Error(), "invalid update checksum signature") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyUpdateChecksumCommandRejectsWrongAsset(t *testing.T) {

	publicKey, privateKey := newTestSigningKey(t)

	const signedAsset = "fbs-interlock-gateway-cluster-linux-amd64"

	checksumData := []byte(
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef  " +
			signedAsset + "\n",
	)
	signature := ed25519.Sign(privateKey, checksumData)

	withTestPublicKey(t, publicKey)

	checksumPath := writeTestFile(t, "asset.sha256", checksumData)
	signaturePath := writeTestFile(t, "asset.sha256.sig", signature)

	_, err := VerifyUpdateChecksumCommand([]string{
		"--checksum", checksumPath,
		"--signature", signaturePath,
		"--asset", "fbs-interlock-gateway-cluster-linux-arm64",
	})
	if err == nil {
		t.Fatal("VerifyUpdateChecksumCommand() succeeded for the wrong asset")
	}

	if !strings.Contains(err.Error(), `signed checksum is for asset "fbs-interlock-gateway-cluster-linux-amd64"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyUpdateChecksumCommandRejectsInvalidSignatureLength(t *testing.T) {

	publicKey, _ := newTestSigningKey(t)
	withTestPublicKey(t, publicKey)

	const asset = "fbs-interlock-gateway-cluster-linux-amd64"

	checksumData := []byte(
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef  " +
			asset + "\n",
	)

	checksumPath := writeTestFile(t, "asset.sha256", checksumData)
	signaturePath := writeTestFile(t, "asset.sha256.sig", []byte("too short"))

	_, err := VerifyUpdateChecksumCommand([]string{
		"--checksum", checksumPath,
		"--signature", signaturePath,
		"--asset", asset,
	})
	if err == nil {
		t.Fatal("VerifyUpdateChecksumCommand() succeeded with an invalid signature length")
	}

	if !strings.Contains(err.Error(), "invalid Ed25519 signature length") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseSignedChecksum(t *testing.T) {
	t.Parallel()

	const validSHA = "ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789"

	tests := []struct {
		name      string
		input     string
		wantSHA   string
		wantAsset string
		wantErr   string
	}{
		{
			name:      "valid text mode checksum",
			input:     validSHA + "  fbs-interlock-gateway-cluster-linux-amd64\n",
			wantSHA:   strings.ToLower(validSHA),
			wantAsset: "fbs-interlock-gateway-cluster-linux-amd64",
		},
		{
			name:      "valid binary mode marker",
			input:     validSHA + " *fbs-interlock-gateway-cluster-linux-amd64\n",
			wantSHA:   strings.ToLower(validSHA),
			wantAsset: "fbs-interlock-gateway-cluster-linux-amd64",
		},
		{
			name:    "empty file",
			input:   "",
			wantErr: "signed checksum file is empty",
		},
		{
			name:    "multiple lines",
			input:   validSHA + "  first\n" + validSHA + "  second\n",
			wantErr: "must contain exactly one line",
		},
		{
			name:    "missing asset",
			input:   validSHA + "\n",
			wantErr: "must contain exactly a SHA-256 digest and asset filename",
		},
		{
			name:    "too many fields",
			input:   validSHA + "  asset extra\n",
			wantErr: "must contain exactly a SHA-256 digest and asset filename",
		},
		{
			name:    "invalid digest",
			input:   "not-a-sha256  asset\n",
			wantErr: "invalid SHA-256 digest",
		},
		{
			name:    "short digest",
			input:   strings.Repeat("a", 62) + "  asset\n",
			wantErr: "invalid SHA-256 digest",
		},
		{
			name:    "path separator slash",
			input:   validSHA + "  path/asset\n",
			wantErr: "must be a base filename",
		},
		{
			name:    "path separator backslash",
			input:   validSHA + "  path\\asset\n",
			wantErr: "must be a base filename",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotSHA, gotAsset, err := parseSignedChecksum([]byte(tt.input))

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("parseSignedChecksum() succeeded; want error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseSignedChecksum() error = %q, want substring %q", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("parseSignedChecksum() error = %v", err)
			}

			if gotSHA != tt.wantSHA {
				t.Fatalf("parseSignedChecksum() SHA = %q, want %q", gotSHA, tt.wantSHA)
			}

			if gotAsset != tt.wantAsset {
				t.Fatalf("parseSignedChecksum() asset = %q, want %q", gotAsset, tt.wantAsset)
			}
		})
	}
}

func TestParseUpdateSigningPublicKeyRejectsInvalidPEM(t *testing.T) {
	original := append([]byte(nil), updateSigningPublicKeyPEM...)
	t.Cleanup(func() {
		updateSigningPublicKeyPEM = original
	})

	updateSigningPublicKeyPEM = []byte("not PEM")

	_, err := parseUpdateSigningPublicKey()
	if err == nil {
		t.Fatal("parseUpdateSigningPublicKey() succeeded with invalid PEM")
	}

	if !strings.Contains(err.Error(), "decode embedded update signing public key") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseUpdateSigningPublicKeyRejectsTrailingData(t *testing.T) {
	publicKey, _ := newTestSigningKey(t)
	publicPEM := encodePublicKeyPEM(t, publicKey)

	original := append([]byte(nil), updateSigningPublicKeyPEM...)
	t.Cleanup(func() {
		updateSigningPublicKeyPEM = original
	})

	updateSigningPublicKeyPEM = append(publicPEM, []byte("unexpected")...)

	_, err := parseUpdateSigningPublicKey()
	if err == nil {
		t.Fatal("parseUpdateSigningPublicKey() succeeded with trailing data")
	}

	if !strings.Contains(err.Error(), "contains trailing data") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseUpdateSigningPublicKeyRejectsWrongKeyType(t *testing.T) {
	block := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: []byte("not a public key"),
	})

	original := append([]byte(nil), updateSigningPublicKeyPEM...)
	t.Cleanup(func() {
		updateSigningPublicKeyPEM = original
	})

	updateSigningPublicKeyPEM = block

	_, err := parseUpdateSigningPublicKey()
	if err == nil {
		t.Fatal("parseUpdateSigningPublicKey() succeeded with the wrong PEM type")
	}

	if !strings.Contains(err.Error(), `expected PUBLIC KEY`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyUpdateChecksumCommandRequiredFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing checksum",
			args:    []string{"--signature", "sig", "--asset", "asset"},
			wantErr: "--checksum is required",
		},
		{
			name:    "missing signature",
			args:    []string{"--checksum", "sum", "--asset", "asset"},
			wantErr: "--signature is required",
		},
		{
			name:    "missing asset",
			args:    []string{"--checksum", "sum", "--signature", "sig"},
			wantErr: "--asset is required",
		},
		{
			name:    "unexpected positional argument",
			args:    []string{"--checksum", "sum", "--signature", "sig", "--asset", "asset", "extra"},
			wantErr: "unexpected arguments: extra",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := VerifyUpdateChecksumCommand(tt.args)
			if err == nil {
				t.Fatalf("VerifyUpdateChecksumCommand() succeeded; want error containing %q", tt.wantErr)
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("VerifyUpdateChecksumCommand() error = %q, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func newTestSigningKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey() error = %v", err)
	}

	return publicKey, privateKey
}

func withTestPublicKey(t *testing.T, publicKey ed25519.PublicKey) {
	t.Helper()

	original := append([]byte(nil), updateSigningPublicKeyPEM...)
	t.Cleanup(func() {
		updateSigningPublicKeyPEM = original
	})

	updateSigningPublicKeyPEM = encodePublicKeyPEM(t, publicKey)
}

func encodePublicKeyPEM(t *testing.T, publicKey ed25519.PublicKey) []byte {
	t.Helper()

	der, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		t.Fatalf("x509.MarshalPKIXPublicKey() error = %v", err)
	}

	return pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: der,
	})
}

func writeTestFile(t *testing.T, name string, data []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", path, err)
	}

	return path
}

func TestParseSignedChecksumRejectsNonSHA256LengthEvenWhenHex(t *testing.T) {
	t.Parallel()

	tooShort := hex.EncodeToString(make([]byte, 31))
	_, _, err := parseSignedChecksum([]byte(tooShort + "  asset\n"))
	if err == nil {
		t.Fatal("parseSignedChecksum() succeeded with a 31-byte digest")
	}

	if !strings.Contains(err.Error(), "invalid SHA-256 digest") {
		t.Fatalf("unexpected error: %v", err)
	}
}
