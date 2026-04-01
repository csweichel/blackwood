package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

func TestGenerateSecret(t *testing.T) {
	secret, uri, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	if secret == "" {
		t.Fatal("expected non-empty secret")
	}
	if !strings.HasPrefix(uri, "otpauth://totp/") {
		t.Errorf("URI = %q, want otpauth:// prefix", uri)
	}
	if !strings.Contains(uri, "secret="+secret) {
		t.Errorf("URI %q does not contain secret", uri)
	}
}

func TestValidateCode_Correct(t *testing.T) {
	secret, _, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}

	// Generate a valid code for the current time.
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}

	if !ValidateCode(secret, code) {
		t.Error("ValidateCode returned false for a valid code")
	}
}

func TestValidateCode_Wrong(t *testing.T) {
	secret, _, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}

	if ValidateCode(secret, "000000") {
		t.Error("ValidateCode returned true for an invalid code")
	}
}
