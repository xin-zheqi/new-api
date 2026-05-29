package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeTokenGroup(t *testing.T) {
	require.Equal(t, "", NormalizeTokenGroup(""))
	require.Equal(t, "default", NormalizeTokenGroup(" default "))
	require.Equal(t, "default,vip", NormalizeTokenGroup("default, vip,default,, "))
}

func TestTokenGroupHelpers(t *testing.T) {
	require.True(t, (&Token{Group: "auto"}).IsAutoGroup())
	require.False(t, (&Token{Group: "auto,default"}).IsAutoGroup())
	require.True(t, (&Token{Group: "default,vip"}).IsMultiGroup())
	require.False(t, (&Token{Group: "vip"}).IsMultiGroup())
}
