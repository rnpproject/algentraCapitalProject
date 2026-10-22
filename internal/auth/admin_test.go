package auth

import "testing"

func TestVerifyAdminPasswordMatchesPythonBcryptHash(t *testing.T) {
	if !VerifyAdminPassword("algentra-dummy-admin-password", string(dummyAdminHash)) {
		t.Fatal("Python bcrypt hash was not accepted")
	}
	if VerifyAdminPassword("wrong-password", string(dummyAdminHash)) {
		t.Fatal("wrong password was accepted")
	}
}

func TestClientPasswordHashUsesPythonPrehashFormat(t *testing.T) {
	hash, err := HashClientPassword("correct-horse-battery-staple")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyClientPassword("correct-horse-battery-staple", hash) {
		t.Fatal("hash did not verify")
	}
	if VerifyClientPassword("different-password", hash) {
		t.Fatal("different password was accepted")
	}
}
