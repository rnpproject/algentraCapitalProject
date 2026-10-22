package security

import "testing"

const (
	testAppSecret       = "test-app-secret-at-least-thirty-two"
	pythonFernetFixture = "gAAAAABq0iw2FdrwFrNNMpnSPCzmUOt4J65cTAtBHKr7S3WuHu39XO3hRDChj4UVPll7zcQ__xsIh15pR19ork1gtqfoT32LF2N2DwZ0AitqGJUdexK9bTE="
)

func TestDecryptPythonFernetFixture(t *testing.T) {
	got, err := DecryptManagedCopyPassword(testAppSecret, pythonFernetFixture)
	if err != nil {
		t.Fatal(err)
	}
	if got != "broker-demo-password" {
		t.Fatalf("decrypted value = %q", got)
	}
}

func TestEncryptAndDecryptManagedCopyPassword(t *testing.T) {
	token, err := EncryptManagedCopyPassword(testAppSecret, "new-demo-password")
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecryptManagedCopyPassword(testAppSecret, token)
	if err != nil || got != "new-demo-password" {
		t.Fatalf("round trip = %q, %v", got, err)
	}
	if _, err := DecryptManagedCopyPassword(testAppSecret, token[:len(token)-4]+"AAAA"); err == nil {
		t.Fatal("tampered token was accepted")
	}
}
