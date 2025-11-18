package conversion

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	v2 "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	v2alpha1 "github.com/michelangelo-ai/michelangelo/proto/api/v2alpha1"
)

// ============================================================================
// Helper Functions for Creating Test Objects
// ============================================================================

// createTestV2Alpha1Project creates a v2alpha1 Project (spoke) with test data.
// This represents a spoke version with:
// - [DEMO CHANGE 3] Renamed field: project_description (instead of description)
// - [DEMO CHANGE 4] Spoke-only field: legacy_alpha_config
// - [DEMO CHANGE 1,2,5,6] Missing fields: type_info, ext, last_update_time (excluded due to type incompatibility)
func createTestV2Alpha1Project() *v2alpha1.Project {
	return &v2alpha1.Project{
		Spec: v2alpha1.ProjectSpec{
			ProjectDescription: "ML Classification Project", // [DEMO CHANGE 3] Renamed field
			Owner: &v2alpha1.OwnerInfo{
				OwningTeam:  "ml-team-uuid-123",
				Owners:      []string{"alice", "bob"},
				OwnerGroups: []string{"ml-engineers"},
			},
			Tier:    2,
			GitRepo: "https://github.com/company/ml-project",
			RootDir: "/ml-classification",
			Commit: &v2alpha1.CommitInfo{
				GitRef: "abc123def456",
				Branch: "main",
			},
			SupportingLinks: map[string]string{
				"docs": "https://docs.company.com/ml-project",
			},
			RetentionConfig: &v2alpha1.RetentionConfig{
				OnlineDeploymentRetention: &v2alpha1.Retention{
					RetentionInDays: 30,
				},
				NotificationControl: v2alpha1.NOTIFICATION_CONTROL_PROJECT_OWNER_ONLY,
			},
			LegacyAlphaConfig: "alpha-specific-config", // [DEMO CHANGE 4] Spoke-only field
		},
		Status: v2alpha1.ProjectStatus{
			State:        v2alpha1.PROJECT_STATE_READY,
			Phase:        v2alpha1.PROJECT_PHASE_DEVELOPMENT,
			ErrorMessage: "",
		},
	}
}

// createTestV2Project creates a v2 Project (hub) with test data.
// This represents a hub version with:
// - [DEMO CHANGE 3] Standard field name: description
// - [DEMO CHANGE 1] Hub-only field: type_info (can be populated)
// - [DEMO CHANGE 2,5,6] Fields that cause type incompatibility (set to nil in conversion tests)
func createTestV2Project() *v2.Project {
	return &v2.Project{
		Spec: v2.ProjectSpec{
			Description: "ML Classification Project", // [DEMO CHANGE 3] Standard field name
			Owner: &v2.OwnerInfo{
				OwningTeam:  "ml-team-uuid-123",
				Owners:      []string{"alice", "bob"},
				OwnerGroups: []string{"ml-engineers"},
			},
			Tier:    2,
			GitRepo: "https://github.com/company/ml-project",
			RootDir: "/ml-classification",
			Commit: &v2.CommitInfo{
				GitRef: "abc123def456",
				Branch: "main",
			},
			SupportingLinks: map[string]string{
				"docs": "https://docs.company.com/ml-project",
			},
			RetentionConfig: &v2.RetentionConfig{
				OnlineDeploymentRetention: &v2.Retention{
					RetentionInDays: 30,
				},
				NotificationControl: v2.NOTIFICATION_CONTROL_PROJECT_OWNER_ONLY,
			},
			TypeInfo: nil, // [DEMO CHANGE 1] Hub-only field (not in v2alpha1)
			Ext:      nil, // [DEMO CHANGE 2] Excluded due to type incompatibility
		},
		Status: v2.ProjectStatus{
			State:          v2.PROJECT_STATE_READY,
			Phase:          v2.PROJECT_PHASE_DEVELOPMENT,
			ErrorMessage:   "",
			LastUpdateTime: nil, // [DEMO CHANGE 6] Excluded due to type incompatibility
			Ext:            nil, // [DEMO CHANGE 5] Excluded due to type incompatibility
		},
	}
}

// createTestV2ProjectWithHubOnlyFields creates a v2 Project (hub) with hub-only fields populated.
// This is used to test that hub-only fields are NOT copied to spoke during ConvertFrom.
func createTestV2ProjectWithHubOnlyFields() *v2.Project {
	return &v2.Project{
		Spec: v2.ProjectSpec{
			Description: "Hub Project with Extra Fields",
			Owner: &v2.OwnerInfo{
				OwningTeam:  "hub-team-uuid",
				Owners:      []string{"charlie"},
				OwnerGroups: []string{"platform-team"},
			},
			Tier:    3,
			GitRepo: "https://github.com/company/hub-project",
			RootDir: "/hub-project",
			Commit: &v2.CommitInfo{
				GitRef: "hub789xyz",
				Branch: "production",
			},
			SupportingLinks: map[string]string{
				"wiki": "https://wiki.company.com",
			},
			RetentionConfig: &v2.RetentionConfig{
				OfflineDeploymentRetention: &v2.Retention{
					RetentionInDays: 90,
				},
				NotificationControl: v2.NOTIFICATION_CONTROL_PROJECT_OWNER_AND_USER_TEAM,
			},
			// [DEMO CHANGE 1] Hub-only field - should NOT be copied to spoke
			TypeInfo: &v2.ProjectTypeInfo{
				IsCoreMl:       true,
				IsGenerativeAi: false,
			},
			// [DEMO CHANGE 2] Type incompatibility field - not set to avoid issues
			Ext: nil,
		},
		Status: v2.ProjectStatus{
			State:          v2.PROJECT_STATE_READY,
			Phase:          v2.PROJECT_PHASE_PRODUCTION,
			ErrorMessage:   "hub status message",
			LastUpdateTime: nil, // [DEMO CHANGE 6] Type incompatibility - not set
			Ext:            nil, // [DEMO CHANGE 5] Type incompatibility - not set
		},
	}
}

// createExpectedV2Alpha1FromHub creates the expected v2alpha1 Project after converting from v2.
// This matches createTestV2ProjectWithHubOnlyFields but with:
// - Field rename: description → project_description
// - TypeInfo removed (hub-only field)
// - LegacyAlphaConfig is empty (spoke-only field, not in hub)
func createExpectedV2Alpha1FromHub() *v2alpha1.Project {
	return &v2alpha1.Project{
		Spec: v2alpha1.ProjectSpec{
			ProjectDescription: "Hub Project with Extra Fields", // [DEMO CHANGE 3] Field renamed
			Owner: &v2alpha1.OwnerInfo{
				OwningTeam:  "hub-team-uuid",
				Owners:      []string{"charlie"},
				OwnerGroups: []string{"platform-team"},
			},
			Tier:    3,
			GitRepo: "https://github.com/company/hub-project",
			RootDir: "/hub-project",
			Commit: &v2alpha1.CommitInfo{
				GitRef: "hub789xyz",
				Branch: "production",
			},
			SupportingLinks: map[string]string{
				"wiki": "https://wiki.company.com",
			},
			RetentionConfig: &v2alpha1.RetentionConfig{
				OfflineDeploymentRetention: &v2alpha1.Retention{
					RetentionInDays: 90,
				},
				NotificationControl: v2alpha1.NOTIFICATION_CONTROL_PROJECT_OWNER_AND_USER_TEAM,
			},
			LegacyAlphaConfig: "", // [DEMO CHANGE 4] Spoke-only field - not in hub, so empty
		},
		Status: v2alpha1.ProjectStatus{
			State:        v2alpha1.PROJECT_STATE_READY,
			Phase:        v2alpha1.PROJECT_PHASE_PRODUCTION,
			ErrorMessage: "hub status message",
			// Note: LastUpdateTime and Ext are not in v2alpha1 (type incompatibility)
		},
	}
}

// ============================================================================
// Test Cases
// ============================================================================

// TestProjectConversion verifies that v2 and v2alpha1 implement the correct interfaces
// for Kubernetes multi-version CRD support.
func TestProjectConversion(t *testing.T) {
	// Verify v2 (hub) implements Hub interface
	v2Project := &v2.Project{}
	assert.Implements(t, (*conversion.Hub)(nil), v2Project,
		"v2.Project should implement conversion.Hub interface")

	// Verify v2alpha1 (spoke) implements Convertible interface
	v2alpha1Project := &v2alpha1.Project{}
	assert.Implements(t, (*conversion.Convertible)(nil), v2alpha1Project,
		"v2alpha1.Project should implement conversion.Convertible interface")
}

// TestProjectConvertToHub tests conversion from v2alpha1 (spoke) to v2 (hub).
//
// Test Flow:
//   Input:    v2alpha1.Project (spoke)
//   Convert:  v2alpha1 → v2 (using ConvertTo)
//   Expected: v2.Project (hub)
//   Verify:   actual == expected
//
// Demo Changes Tested:
// - [DEMO CHANGE 3] Field rename: project_description → description
// - [DEMO CHANGE 4] Spoke-only field legacy_alpha_config is NOT copied to hub
// - [DEMO CHANGE 1,2,5,6] Excluded fields remain nil in hub
func TestProjectConvertToHub(t *testing.T) {
	// Input: Create v2alpha1 Project (spoke) with test data
	input := createTestV2Alpha1Project()

	// Expected: Create the expected v2 Project (hub) after conversion
	// Note: legacy_alpha_config should NOT be copied (spoke-only field)
	expected := createTestV2Project()

	// Convert: v2alpha1 → v2
	actual := &v2.Project{}
	err := input.ConvertTo(actual)

	// Verify: No errors and actual matches expected
	assert.NoError(t, err, "ConvertTo should not return error")
	assert.Equal(t, expected, actual, "Converted v2 Project should match expected")
}

// TestProjectConvertFromHub tests conversion from v2 (hub) to v2alpha1 (spoke).
//
// Test Flow:
//   Input:    v2.Project (hub) with hub-only fields
//   Convert:  v2 → v2alpha1 (using ConvertFrom)
//   Expected: v2alpha1.Project (spoke) without hub-only fields
//   Verify:   actual == expected
//
// Demo Changes Tested:
// - [DEMO CHANGE 3] Field rename: description → project_description
// - [DEMO CHANGE 1] Hub-only field type_info is NOT copied to spoke
// - [DEMO CHANGE 4] Spoke-only field legacy_alpha_config is empty (not in hub)
// - [DEMO CHANGE 2,5,6] Type incompatibility fields are not present in spoke
func TestProjectConvertFromHub(t *testing.T) {
	// Input: Create v2 Project (hub) with hub-only fields populated
	input := createTestV2ProjectWithHubOnlyFields()

	// Expected: Create expected v2alpha1 Project after conversion
	// - TypeInfo should be excluded (hub-only field)
	// - LegacyAlphaConfig should be empty (spoke-only field, not in hub)
	expected := createExpectedV2Alpha1FromHub()

	// Convert: v2 → v2alpha1
	actual := &v2alpha1.Project{}
	err := actual.ConvertFrom(input)

	// Verify: No errors and actual matches expected
	assert.NoError(t, err, "ConvertFrom should not return error")
	assert.Equal(t, expected, actual, "Converted v2alpha1 Project should match expected")
}

// TestProjectRoundTripConversion tests bidirectional conversion: v2alpha1 → v2 → v2alpha1.
//
// Test Flow:
//   Start:    v2alpha1.Project (spoke) with spoke-only field
//   Step 1:   v2alpha1 → v2 (spoke to hub, loses legacy_alpha_config)
//   Step 2:   v2 → v2alpha1 (hub back to spoke)
//   Verify:   final == start (except spoke-only field is lost)
//
// Demo Changes Tested:
// - [DEMO CHANGE 4] Spoke-only field is lost during round-trip (expected behavior)
// - [DEMO CHANGE 3] Field rename works bidirectionally
// - All other fields are preserved
func TestProjectRoundTripConversion(t *testing.T) {
	// Start: Create original v2alpha1 Project with spoke-only field
	original := createTestV2Alpha1Project()
	original.Spec.LegacyAlphaConfig = "will-be-lost-in-roundtrip"

	// Step 1: Convert v2alpha1 → v2 (spoke to hub)
	hub := &v2.Project{}
	err := original.ConvertTo(hub)
	assert.NoError(t, err, "ConvertTo should not return error")

	// Step 2: Convert v2 → v2alpha1 (hub back to spoke)
	result := &v2alpha1.Project{}
	err = result.ConvertFrom(hub)
	assert.NoError(t, err, "ConvertFrom should not return error")

	// Expected: Same as original but legacy_alpha_config is lost (spoke-only field)
	expected := createTestV2Alpha1Project()
	expected.Spec.LegacyAlphaConfig = "" // Lost during round-trip

	// Verify: Result matches expected
	assert.Equal(t, expected, result,
		"Round-trip conversion should preserve all fields except spoke-only fields")
}
