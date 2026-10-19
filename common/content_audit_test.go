package common

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAuditEncryptionBindsOwnerPurposeAndRejectsTampering(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "content-audit-test-secret-0123456789")
	t.Setenv("SESSION_SECRET", "")
	previous := GetContentAuditSettings()
	require.NoError(t, ApplyContentAuditSettings(`{"enabled":true,"retention_days":7}`))
	t.Cleanup(func() {
		value, err := Marshal(previous)
		if err == nil {
			_ = ApplyContentAuditSettings(string(value))
		}
	})
	ciphertext, err := SealContentAudit([]byte("private text"), "hits:1:record")
	require.NoError(t, err)
	assert.NotContains(t, ciphertext, "private text")
	plain, err := OpenContentAudit(ciphertext, "hits:1:record")
	require.NoError(t, err)
	assert.Equal(t, "private text", string(plain))
	_, err = OpenContentAudit(ciphertext, "hits:2:record")
	require.Error(t, err)
	_, err = OpenContentAudit(ciphertext, "rules:1")
	require.Error(t, err)
	_, err = OpenContentAudit(ciphertext[:len(ciphertext)-4]+"AAAA", "hits:1:record")
	require.Error(t, err)
}
func TestAuditConfigurationRequiresEnvironmentSecret(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "")
	t.Setenv("SESSION_SECRET", "")
	previousCryptoSecret := CryptoSecret
	CryptoSecret = ""
	t.Cleanup(func() { CryptoSecret = previousCryptoSecret })
	previous := GetContentAuditSettings()
	t.Cleanup(func() {
		value, err := Marshal(previous)
		if err == nil {
			_ = ApplyContentAuditSettings(string(value))
		}
	})
	require.NoError(t, ApplyContentAuditSettings(`{"enabled":false,"retention_days":7}`))
	normalized, err := NormalizeContentAuditSettings(`{"enabled":true,"retention_days":7}`)
	require.NoError(t, err)
	settings, err := ParseContentAuditSettings(normalized)
	require.NoError(t, err)
	assert.True(t, settings.Enabled)
	assert.Error(t, ApplyContentAuditSettings(normalized))
	t.Setenv("CRYPTO_SECRET", "persistent-test-secret-0123456789")
	require.NoError(t, ApplyContentAuditSettings(normalized))
	sealed, err := SealContentAudit([]byte("persisted"), "rules:1")
	require.NoError(t, err)
	opened, err := OpenContentAudit(sealed, "rules:1")
	require.NoError(t, err)
	assert.Equal(t, "persisted", string(opened))

	normalized, err = NormalizeContentAuditSettings(`{"enabled":true,"retention_days":7,"secret":"legacy-secret"}`)
	require.NoError(t, err)
	assert.NotContains(t, normalized, "legacy-secret")
	_, err = ParseContentAuditSettings(`{"enabled":true,"retention_days":0}`)
	require.Error(t, err)
	_, err = ParseContentAuditSettings(`{"enabled":true,"retention_days":7}`)
	require.NoError(t, err)
}

func TestPublicContentAuditSettingsNeverReturnsSecret(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "content-audit-public-test-secret-0123456789")
	t.Setenv("SESSION_SECRET", "")
	value := `{"enabled":true,"retention_days":7,"secret":"content-audit-public-test-secret-0123456789"}`
	public := PublicContentAuditSettings(value)
	assert.NotContains(t, public, "content-audit-public-test-secret")
	settings, err := ParseContentAuditSettings(public)
	require.NoError(t, err)
	assert.True(t, settings.Enabled)
}
