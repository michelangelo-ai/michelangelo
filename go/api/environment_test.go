package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsBranchAllowedForEnvironment(t *testing.T) {
	tests := []struct {
		name               string
		environment        string
		branch             string
		restrictedBranches map[string][]string
		want               bool
	}{
		{
			name:               "nil map — always allowed",
			environment:        "production",
			branch:             "feature-x",
			restrictedBranches: nil,
			want:               true,
		},
		{
			name:               "empty map — always allowed",
			environment:        "production",
			branch:             "feature-x",
			restrictedBranches: map[string][]string{},
			want:               true,
		},
		{
			name:               "environment absent from map — always allowed",
			environment:        "production",
			branch:             "feature-x",
			restrictedBranches: map[string][]string{"staging": {"main"}},
			want:               true,
		},
		{
			name:               "environment present with nil slice — always allowed",
			environment:        "production",
			branch:             "feature-x",
			restrictedBranches: map[string][]string{"production": nil},
			want:               true,
		},
		{
			name:               "environment present with empty slice — always allowed",
			environment:        "production",
			branch:             "feature-x",
			restrictedBranches: map[string][]string{"production": {}},
			want:               true,
		},
		{
			name:               "matching branch — allowed",
			environment:        "production",
			branch:             "main",
			restrictedBranches: map[string][]string{"production": {"main", "master"}},
			want:               true,
		},
		{
			name:               "matching second branch entry — allowed",
			environment:        "production",
			branch:             "master",
			restrictedBranches: map[string][]string{"production": {"main", "master"}},
			want:               true,
		},
		{
			name:               "non-matching branch — rejected",
			environment:        "production",
			branch:             "feature-x",
			restrictedBranches: map[string][]string{"production": {"main", "master"}},
			want:               false,
		},
		{
			name:               "case sensitivity — exact match required",
			environment:        "production",
			branch:             "Main",
			restrictedBranches: map[string][]string{"production": {"main"}},
			want:               false,
		},
		{
			name:               "case sensitivity on environment key",
			environment:        "Production",
			branch:             "feature-x",
			restrictedBranches: map[string][]string{"production": {"main"}},
			want:               true, // "Production" is absent from the map, so unrestricted
		},
		{
			name:        "unrelated environment key does not affect lookup for another key",
			environment: "production",
			branch:      "feature-x",
			restrictedBranches: map[string][]string{
				"staging":    {"main", "release"},
				"production": {"main"},
			},
			want: false, // "production" key restricts to "main"; "staging" is irrelevant
		},
		{
			name:        "unrelated key present, queried environment absent — allowed",
			environment: "development",
			branch:      "feature-x",
			restrictedBranches: map[string][]string{
				"production": {"main"},
				"staging":    {"release"},
			},
			want: true, // "development" is not a key in the map
		},
		{
			name:               "non-standard environment name — works identically",
			environment:        "prod",
			branch:             "release",
			restrictedBranches: map[string][]string{"prod": {"main"}},
			want:               false,
		},
		{
			name:               "non-standard environment name — matching branch",
			environment:        "prod",
			branch:             "main",
			restrictedBranches: map[string][]string{"prod": {"main"}},
			want:               true,
		},
		{
			name:               "empty branch string — non-matching",
			environment:        "production",
			branch:             "",
			restrictedBranches: map[string][]string{"production": {"main"}},
			want:               false,
		},
		{
			name:               "empty environment string — absent from map, allowed",
			environment:        "",
			branch:             "main",
			restrictedBranches: map[string][]string{"production": {"main"}},
			want:               true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsBranchAllowedForEnvironment(tt.environment, tt.branch, tt.restrictedBranches)
			assert.Equal(t, tt.want, got)
		})
	}
}
