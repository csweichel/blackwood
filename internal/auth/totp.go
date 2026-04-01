package auth

import (
	"fmt"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const (
	totpIssuer      = "Blackwood"
	totpAccountName = "user"
)

// GenerateSecret creates a new TOTP secret and returns the raw secret string
// and the otpauth:// URI for QR code generation.
func GenerateSecret() (secret string, uri string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      totpIssuer,
		AccountName: totpAccountName,
	})
	if err != nil {
		return "", "", fmt.Errorf("generate TOTP key: %w", err)
	}
	return key.Secret(), key.URL(), nil
}

// ValidateCode checks a 6-digit TOTP code against the given secret.
func ValidateCode(secret, code string) bool {
	valid, _ := totp.ValidateCustom(code, secret, time.Now(), totp.ValidateOpts{
		Period:    30,
		Skew:     1,
		Digits:   otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	return valid
}
