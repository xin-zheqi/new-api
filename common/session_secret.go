package common

import "errors"

const SessionSecretMinLength = 32

func ValidateSessionSecret(secret string) error {
	if len(secret) < SessionSecretMinLength {
		return errors.New("SESSION_SECRET must contain at least 32 characters of random data")
	}
	return nil
}
