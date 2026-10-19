package common

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"sync/atomic"
)

const ContentAuditOption = "ContentAuditSettings"

type ContentAuditSettings struct {
	Enabled       bool `json:"enabled"`
	RetentionDays int  `json:"retention_days"`
}

var contentAuditSettings atomic.Pointer[ContentAuditSettings]

func GetContentAuditSettings() ContentAuditSettings {
	if current := contentAuditSettings.Load(); current != nil {
		return *current
	}
	return ContentAuditSettings{RetentionDays: 7}
}

func ParseContentAuditSettings(value string) (ContentAuditSettings, error) {
	var settings ContentAuditSettings
	if err := UnmarshalJsonStr(value, &settings); err != nil {
		return settings, errors.New("invalid content audit settings")
	}
	if settings.RetentionDays < 1 || settings.RetentionDays > 90 {
		return settings, errors.New("content audit retention must be between 1 and 90 days")
	}
	return settings, nil
}

// ValidateContentAuditSettings validates both the persisted shape and the
// environment requirement needed when the feature is enabled. Keeping this
// check separate lets option writes fail before a bad value reaches storage.
func ValidateContentAuditSettings(value string) error {
	settings, err := ParseContentAuditSettings(value)
	if err != nil {
		return err
	}
	if settings.Enabled && contentAuditEnvironmentSecret() == "" {
		return errors.New("configure a persistent CRYPTO_SECRET or SESSION_SECRET before enabling content audit")
	}
	return nil
}

func contentAuditEnvironmentSecret() string {
	for _, name := range []string{"CRYPTO_SECRET", "SESSION_SECRET"} {
		if secret := strings.TrimSpace(os.Getenv(name)); secret != "" {
			return secret
		}
	}
	return ""
}

func currentContentAuditSecret() string {
	return contentAuditEnvironmentSecret()
}

// NormalizeContentAuditSettings prepares the value written to the options table.
// Encryption keys are intentionally excluded from persisted settings. Older
// installations may still have a secret field; unmarshalling into the current
// struct drops it on the next save.
func NormalizeContentAuditSettings(value string) (string, error) {
	settings, err := ParseContentAuditSettings(value)
	if err != nil {
		return "", err
	}
	encoded, err := Marshal(settings)
	if err != nil {
		return "", errors.New("cannot encode content audit settings")
	}
	return string(encoded), nil
}

// PublicContentAuditSettings removes legacy secret fields before an option is
// returned to an administrator's browser. New settings never contain key
// material, but this keeps old option rows from leaking it to the frontend.
func PublicContentAuditSettings(value string) string {
	encoded, err := NormalizeContentAuditSettings(value)
	if err != nil {
		return value
	}
	return string(encoded)
}

func ApplyContentAuditSettings(value string) error {
	settings, err := ParseContentAuditSettings(value)
	if err != nil {
		return err
	}
	if settings.Enabled && contentAuditEnvironmentSecret() == "" {
		return errors.New("configure a persistent CRYPTO_SECRET or SESSION_SECRET before enabling content audit")
	}
	contentAuditSettings.Store(&settings)
	return nil
}

// Bind ciphertext to its owner and purpose: swapping database rows must never
// expose another user's rules or matched text. Ciphertext is not a server-admin
// isolation boundary; the server necessarily holds the decryption key.
func contentAuditCipher() (cipher.AEAD, error) {
	secret := currentContentAuditSecret()
	if secret == "" {
		return nil, errors.New("content audit encryption key is not configured")
	}
	key := sha256.Sum256([]byte("new-api/content-audit/v1/" + secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func SealContentAudit(data []byte, owner string) (string, error) {
	aead, err := contentAuditCipher()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := aead.Seal(nonce, nonce, data, []byte(owner))
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func OpenContentAudit(data, owner string) ([]byte, error) {
	aead, err := contentAuditCipher()
	if err != nil {
		return nil, err
	}
	sealed, err := base64.StdEncoding.DecodeString(data)
	if err != nil || len(sealed) < aead.NonceSize() {
		return nil, errors.New("invalid encrypted content audit data")
	}
	plain, err := aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], []byte(owner))
	if err != nil {
		return nil, errors.New("content audit data cannot be decrypted; check the persistent encryption secret")
	}
	return plain, nil
}
