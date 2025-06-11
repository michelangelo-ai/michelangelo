package utils

import (
	"fmt"
	"os/user"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDefaultSpiffeIDProvider(t *testing.T) {
	provider := NewDefaultSpiffeIDProvider()
	assert.NotNil(t, provider)
	assert.NotNil(t, provider.lookupFunc)
}

func TestGetSpiffeIDWithValidUser(t *testing.T) {
	mockLookup := func(username string) (*user.User, error) {
		return &user.User{Uid: "12345"}, nil
	}

	provider := &DefaultSpiffeIDProvider{lookupFunc: mockLookup}
	spiffeID := provider.GetSpiffeID("test-user")

	expected := fmt.Sprintf(spiffeIDFormat, "12345")
	assert.Equal(t, expected, spiffeID)
}

func TestGetSpiffeIDWithLookupError(t *testing.T) {
	mockLookup := func(username string) (*user.User, error) {
		return nil, fmt.Errorf("user not found")
	}

	provider := &DefaultSpiffeIDProvider{lookupFunc: mockLookup}
	spiffeID := provider.GetSpiffeID("unknown-user")

	assert.Equal(t, defaultSpiffeID, spiffeID)
}

func TestSpiffeIDFormat(t *testing.T) {
	mockLookup := func(username string) (*user.User, error) {
		return &user.User{Uid: "67890"}, nil
	}

	provider := &DefaultSpiffeIDProvider{lookupFunc: mockLookup}
	spiffeID := provider.GetSpiffeID("format-test-user")

	expected := fmt.Sprintf(spiffeIDFormat, "67890")
	require.Equal(t, expected, spiffeID)
}

func TestGetUserID(t *testing.T) {
	tests := []struct {
		name     string
		ldap     string
		mockUser *user.User
		mockErr  error
		want     string
	}{
		{
			name: "successful lookup",
			ldap: "testuser",
			mockUser: &user.User{
				Uid: "12345",
			},
			mockErr: nil,
			want:    "12345",
		},
		{
			name:     "lookup error",
			ldap:     "nonexistent",
			mockUser: nil,
			mockErr:  fmt.Errorf("user not found"),
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &DefaultSpiffeIDProvider{
				lookupFunc: func(username string) (*user.User, error) {
					return tt.mockUser, tt.mockErr
				},
			}

			got := provider.GetUserID(tt.ldap)
			assert.Equal(t, tt.want, got)
		})
	}
}
