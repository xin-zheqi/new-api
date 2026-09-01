package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateRemoteDatabaseTLS(t *testing.T) {
	tests := []struct {
		name          string
		dsn           string
		postgres      bool
		allowInsecure bool
		wantError     bool
	}{
		{name: "public PostgreSQL without TLS", dsn: "postgresql://user:pass@db.example.com:5432/app", postgres: true, wantError: true},
		{name: "public PostgreSQL disable TLS", dsn: "postgresql://user:pass@db.example.com:5432/app?sslmode=disable", postgres: true, wantError: true},
		{name: "public PostgreSQL require TLS", dsn: "postgresql://user:pass@db.example.com:5432/app?sslmode=require", postgres: true},
		{name: "Docker PostgreSQL service", dsn: "postgresql://user:pass@postgres:5432/app", postgres: true},
		{name: "private PostgreSQL address", dsn: "postgresql://user:pass@10.0.0.12:5432/app", postgres: true},
		{name: "explicit PostgreSQL compatibility override", dsn: "postgresql://user:pass@db.example.com:5432/app", postgres: true, allowInsecure: true},
		{name: "public MySQL without TLS", dsn: "user:pass@tcp(db.example.com:3306)/app?parseTime=true", wantError: true},
		{name: "public MySQL preferred TLS", dsn: "user:pass@tcp(db.example.com:3306)/app?tls=preferred", wantError: true},
		{name: "public MySQL required TLS", dsn: "user:pass@tcp(db.example.com:3306)/app?tls=true", wantError: false},
		{name: "Docker MySQL service", dsn: "user:pass@tcp(mysql:3306)/app", wantError: false},
		{name: "private MySQL address", dsn: "user:pass@tcp(192.168.10.2:3306)/app", wantError: false},
		{name: "explicit MySQL compatibility override", dsn: "user:pass@tcp(db.example.com:3306)/app", allowInsecure: true, wantError: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateRemoteDatabaseTLS(test.dsn, test.postgres, test.allowInsecure)
			if test.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
