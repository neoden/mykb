package httpd

import "testing"

func TestGenerateToken(t *testing.T) {
	t1 := GenerateToken()
	t2 := GenerateToken()

	if t1 == t2 {
		t.Error("Tokens should be unique")
	}
	if len(t1) < 32 {
		t.Errorf("Token length = %d, expected >= 32", len(t1))
	}
}

func TestHashPKCE(t *testing.T) {
	// Test vector from RFC 7636
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	expected := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

	got := HashPKCE(verifier)
	if got != expected {
		t.Errorf("HashPKCE = %q, want %q", got, expected)
	}
}
