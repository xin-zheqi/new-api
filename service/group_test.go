package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetTokenRouteGroupsForMultiGroup(t *testing.T) {
	require.Equal(t, []string{"default", "vip"}, GetTokenRouteGroups("default,vip", "user"))
}

func TestTokenGroupModeHelpers(t *testing.T) {
	require.True(t, TokenGroupIsAuto("auto"))
	require.False(t, TokenGroupIsAuto("auto,default"))
	require.True(t, TokenGroupIsMulti("default,vip"))
	require.False(t, TokenGroupIsMulti("default"))
}
