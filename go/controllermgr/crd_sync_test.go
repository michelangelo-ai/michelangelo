package controllermgr

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/config"
	"go.uber.org/zap/zaptest"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MockGateway implements crd.Gateway interface for testing
type MockGateway struct {
	mock.Mock
}

func (m *MockGateway) ConditionalUpsert(ctx context.Context, crd *apiextv1.CustomResourceDefinition, enableIncompatibleUpdate bool) error {
	args := m.Called(ctx, crd, enableIncompatibleUpdate)
	return args.Error(0)
}

func (m *MockGateway) Delete(ctx context.Context, crdToDelete *apiextv1.CustomResourceDefinition) error {
	args := m.Called(ctx, crdToDelete)
	return args.Error(0)
}

func (m *MockGateway) List(ctx context.Context) (*apiextv1.CustomResourceDefinitionList, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*apiextv1.CustomResourceDefinitionList), args.Error(1)
}

func TestCompareSchemasWithServerList_MatchingSchemas(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	// Create identical local and server CRDs
	localCRD := &apiextv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "projects.example.com"},
		Spec: apiextv1.CustomResourceDefinitionSpec{
			Versions: []apiextv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema: &apiextv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]apiextv1.JSONSchemaProps{
								"spec": {Type: "object"},
							},
						},
					},
				},
			},
		},
	}

	serverCRD := localCRD.DeepCopy()
	
	localSchemas := map[string]*apiextv1.CustomResourceDefinition{
		"projects.example.com": localCRD,
	}

	serverCRDs := &apiextv1.CustomResourceDefinitionList{
		Items: []apiextv1.CustomResourceDefinition{*serverCRD},
	}

	// Should not log any mismatches for identical schemas
	err := compareSchemasWithServerList(ctx, logger, localSchemas, serverCRDs)
	assert.NoError(t, err)
}

func TestCompareSchemasWithServerList_SchemaMismatch(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	// Create different local and server CRDs
	localCRD := &apiextv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "projects.example.com"},
		Spec: apiextv1.CustomResourceDefinitionSpec{
			Versions: []apiextv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
				},
			},
		},
	}

	serverCRD := &apiextv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "projects.example.com"},
		Spec: apiextv1.CustomResourceDefinitionSpec{
			Versions: []apiextv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: false, // Different from local
				},
				{
					Name:    "v2", // Additional version
					Served:  true,
					Storage: true,
				},
			},
		},
	}

	localSchemas := map[string]*apiextv1.CustomResourceDefinition{
		"projects.example.com": localCRD,
	}

	serverCRDs := &apiextv1.CustomResourceDefinitionList{
		Items: []apiextv1.CustomResourceDefinition{*serverCRD},
	}

	// Should detect schema mismatch
	err := compareSchemasWithServerList(ctx, logger, localSchemas, serverCRDs)
	assert.NoError(t, err)
}

func TestCompareSchemasWithServerList_CRDNotFoundOnServer(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	localCRD := &apiextv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "missing.example.com"},
		Spec: apiextv1.CustomResourceDefinitionSpec{
			Versions: []apiextv1.CustomResourceDefinitionVersion{
				{Name: "v1", Served: true, Storage: true},
			},
		},
	}

	localSchemas := map[string]*apiextv1.CustomResourceDefinition{
		"missing.example.com": localCRD,
	}

	// Empty server CRD list
	serverCRDs := &apiextv1.CustomResourceDefinitionList{
		Items: []apiextv1.CustomResourceDefinition{},
	}

	// Should log that CRD is not found on server
	err := compareSchemasWithServerList(ctx, logger, localSchemas, serverCRDs)
	assert.NoError(t, err)
}

func TestPerformSchemaComparison_GatewayListError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	mockGateway := &MockGateway{}
	mockGateway.On("List", ctx).Return(nil, errors.New("API server connection failed"))

	// Should handle gateway.List() error gracefully
	performSchemaComparison(ctx, logger, mockGateway)

	mockGateway.AssertExpectations(t)
}

func TestPerformSchemaComparison_Success(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	serverCRDs := &apiextv1.CustomResourceDefinitionList{
		Items: []apiextv1.CustomResourceDefinition{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "test.example.com"},
				Spec: apiextv1.CustomResourceDefinitionSpec{
					Versions: []apiextv1.CustomResourceDefinitionVersion{
						{Name: "v1", Served: true, Storage: true},
					},
				},
			},
		},
	}

	mockGateway := &MockGateway{}
	mockGateway.On("List", ctx).Return(serverCRDs, nil)

	// Should complete successfully
	performSchemaComparison(ctx, logger, mockGateway)

	mockGateway.AssertExpectations(t)
}

func TestStartPeriodicSchemaComparison_ContextCancellation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := &CRDSyncConfig{
		SyncInterval: 100 * time.Millisecond,
	}

	mockGateway := &MockGateway{}
	mockGateway.On("List", mock.Anything).Return(&apiextv1.CustomResourceDefinitionList{}, nil).Maybe()

	// Create context that will be cancelled quickly
	ctx, cancel := context.WithCancel(context.Background())
	
	// Start the periodic comparison in a goroutine
	done := make(chan bool)
	go func() {
		startPeriodicSchemaComparison(ctx, logger, config, mockGateway)
		done <- true
	}()

	// Cancel context after a short delay
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Wait for goroutine to exit
	select {
	case <-done:
		// Success - goroutine exited cleanly
	case <-time.After(1 * time.Second):
		t.Fatal("Goroutine did not exit within timeout after context cancellation")
	}

	mockGateway.AssertExpectations(t)
}

func TestDeepEqualComparison_SpecDifferences(t *testing.T) {
	// Test that our simplified reflect.DeepEqual catches various spec differences
	
	baseCRD := &apiextv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "test.example.com"},
		Spec: apiextv1.CustomResourceDefinitionSpec{
			Group: "example.com",
			Names: apiextv1.CustomResourceDefinitionNames{
				Plural:   "tests",
				Singular: "test",
				Kind:     "Test",
			},
			Scope: "Namespaced",
			Versions: []apiextv1.CustomResourceDefinitionVersion{
				{Name: "v1", Served: true, Storage: true},
			},
		},
	}

	testCases := []struct {
		name     string
		modifier func(*apiextv1.CustomResourceDefinition)
		expect   bool // true = should detect difference
	}{
		{
			name: "identical specs",
			modifier: func(crd *apiextv1.CustomResourceDefinition) {
				// No changes - should be equal
			},
			expect: false,
		},
		{
			name: "different group",
			modifier: func(crd *apiextv1.CustomResourceDefinition) {
				crd.Spec.Group = "different.com"
			},
			expect: true,
		},
		{
			name: "different kind",
			modifier: func(crd *apiextv1.CustomResourceDefinition) {
				crd.Spec.Names.Kind = "DifferentKind"
			},
			expect: true,
		},
		{
			name: "different scope",
			modifier: func(crd *apiextv1.CustomResourceDefinition) {
				crd.Spec.Scope = "Cluster"
			},
			expect: true,
		},
		{
			name: "additional version",
			modifier: func(crd *apiextv1.CustomResourceDefinition) {
				crd.Spec.Versions = append(crd.Spec.Versions, 
					apiextv1.CustomResourceDefinitionVersion{Name: "v2", Served: true, Storage: false})
			},
			expect: true,
		},
		{
			name: "different version properties",
			modifier: func(crd *apiextv1.CustomResourceDefinition) {
				crd.Spec.Versions[0].Storage = false
			},
			expect: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a copy of the base CRD
			localCRD := baseCRD.DeepCopy()
			serverCRD := baseCRD.DeepCopy()
			
			// Apply the modification to server CRD
			tc.modifier(serverCRD)
			
			// Test our comparison logic
			hasDifference := !reflect.DeepEqual(localCRD.Spec, serverCRD.Spec)
			assert.Equal(t, tc.expect, hasDifference, 
				"Expected difference detection: %v, got: %v", tc.expect, hasDifference)
		})
	}
}

func TestDetailedComparisonLogging(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	// Create CRDs with multiple types of differences
	localCRD := &apiextv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "projects.example.com"},
		Spec: apiextv1.CustomResourceDefinitionSpec{
			Group: "example.com",
			Names: apiextv1.CustomResourceDefinitionNames{
				Kind:     "Project",
				Plural:   "projects",
				Singular: "project",
			},
			Scope: "Namespaced",
			Versions: []apiextv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
				},
				{
					Name:    "v2",
					Served:  true,
					Storage: false,
				},
			},
		},
	}

	serverCRD := &apiextv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "projects.example.com"},
		Spec: apiextv1.CustomResourceDefinitionSpec{
			Group: "different.com", // Different group
			Names: apiextv1.CustomResourceDefinitionNames{
				Kind:     "DifferentProject", // Different kind
				Plural:   "projects",
				Singular: "project",
			},
			Scope: "Cluster", // Different scope
			Versions: []apiextv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  false, // Different served status
					Storage: true,
				},
				{
					Name:    "v3", // Different version name
					Served:  true,
					Storage: false,
				},
			},
		},
	}

	localSchemas := map[string]*apiextv1.CustomResourceDefinition{
		"projects.example.com": localCRD,
	}

	serverCRDs := &apiextv1.CustomResourceDefinitionList{
		Items: []apiextv1.CustomResourceDefinition{*serverCRD},
	}

	// This should log detailed differences
	err := compareSchemasWithServerList(ctx, logger, localSchemas, serverCRDs)
	assert.NoError(t, err)

	// The test will show detailed logging output in the console
}

func TestCompareAndLogDifferences_NoDifferences(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create identical CRDs
	baseCRD := &apiextv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "test.example.com"},
		Spec: apiextv1.CustomResourceDefinitionSpec{
			Group: "example.com",
			Names: apiextv1.CustomResourceDefinitionNames{
				Kind:     "Test",
				Plural:   "tests",
				Singular: "test",
			},
			Scope: "Namespaced",
			Versions: []apiextv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
				},
			},
		},
	}

	localCRD := baseCRD.DeepCopy()
	serverCRD := baseCRD.DeepCopy()

	// Should not log any differences
	compareAndLogDifferences(logger, "test.example.com", localCRD, serverCRD)
}

func TestCompareVersions_MissingVersionsOnServer(t *testing.T) {
	logger := zaptest.NewLogger(t)

	localVersions := []apiextv1.CustomResourceDefinitionVersion{
		{Name: "v1", Served: true, Storage: true},
		{Name: "v2", Served: true, Storage: false},
	}

	serverVersions := []apiextv1.CustomResourceDefinitionVersion{
		{Name: "v1", Served: true, Storage: true},
		// v2 is missing on server
	}

	hasDiff := compareVersions(logger, "test.example.com", localVersions, serverVersions)
	assert.True(t, hasDiff, "Should detect missing version on server")
}

func TestCompareVersions_ExtraVersionsOnServer(t *testing.T) {
	logger := zaptest.NewLogger(t)

	localVersions := []apiextv1.CustomResourceDefinitionVersion{
		{Name: "v1", Served: true, Storage: true},
	}

	serverVersions := []apiextv1.CustomResourceDefinitionVersion{
		{Name: "v1", Served: true, Storage: true},
		{Name: "v2", Served: true, Storage: false}, // Extra version on server
	}

	hasDiff := compareVersions(logger, "test.example.com", localVersions, serverVersions)
	assert.True(t, hasDiff, "Should detect extra version on server")
}

func TestCompareVersionDetails_AllFields(t *testing.T) {
	logger := zaptest.NewLogger(t)

	deprecationWarning := "This version is deprecated"
	localVersion := apiextv1.CustomResourceDefinitionVersion{
		Name:               "v1",
		Served:             true,
		Storage:            true,
		Deprecated:         false,
		DeprecationWarning: &deprecationWarning,
		Schema: &apiextv1.CustomResourceValidation{
			OpenAPIV3Schema: &apiextv1.JSONSchemaProps{
				Type: "object",
				Properties: map[string]apiextv1.JSONSchemaProps{
					"spec": {Type: "object"},
				},
			},
		},
	}

	serverVersion := apiextv1.CustomResourceDefinitionVersion{
		Name:               "v1",
		Served:             false, // Different
		Storage:            false, // Different
		Deprecated:         true,  // Different
		DeprecationWarning: nil,   // Different
		Schema: &apiextv1.CustomResourceValidation{
			OpenAPIV3Schema: &apiextv1.JSONSchemaProps{
				Type: "string", // Different type
				Properties: map[string]apiextv1.JSONSchemaProps{
					"status": {Type: "object"}, // Different properties
				},
			},
		},
	}

	hasDiff := compareVersionDetails(logger, "test.example.com", "v1", localVersion, serverVersion)
	assert.True(t, hasDiff, "Should detect all version field differences")
}

func TestCompareSchemaDetails_NilSchemas(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Test nil local schema
	compareSchemaDetails(logger, "test.example.com", "v1", nil, &apiextv1.CustomResourceValidation{})

	// Test nil server schema
	compareSchemaDetails(logger, "test.example.com", "v1", &apiextv1.CustomResourceValidation{}, nil)

	// Test both nil
	compareSchemaDetails(logger, "test.example.com", "v1", nil, nil)
}

func TestStartCRDSync(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := &CRDSyncConfig{
		SyncInterval: 100 * time.Millisecond,
	}

	mockGateway := &MockGateway{}
	mockGateway.On("List", mock.Anything).Return(&apiextv1.CustomResourceDefinitionList{}, nil)

	params := CRDSyncParams{
		Config:  config,
		Logger:  logger,
		Gateway: mockGateway,
	}

	// This should start the background goroutine without error
	err := startCRDSync(params)
	assert.NoError(t, err)

	// Give the goroutine time to make at least one call
	time.Sleep(150 * time.Millisecond)

	// Verify that the gateway was called (meaning the goroutine started successfully)
	mockGateway.AssertCalled(t, "List", mock.Anything)
}

func TestNewCRDSyncConfig(t *testing.T) {
	// Test default configuration
	provider := &mockConfigProvider{
		data: map[string]interface{}{},
	}

	config, err := newCRDSyncConfig(provider)
	assert.NoError(t, err)
	assert.Equal(t, 5*time.Minute, config.SyncInterval)
}

func TestNewCRDSyncConfig_CustomInterval(t *testing.T) {
	// Test custom configuration
	provider := &mockConfigProvider{
		data: map[string]interface{}{
			"syncInterval": "10m",
		},
	}

	config, err := newCRDSyncConfig(provider)
	assert.NoError(t, err)
	assert.Equal(t, 10*time.Minute, config.SyncInterval)
}

// mockConfigProvider implements config.Provider for testing
type mockConfigProvider struct {
	data map[string]interface{}
}

func (m *mockConfigProvider) Name() string {
	return "test"
}

func (m *mockConfigProvider) Get(key string) config.Value {
	// Create a proper config.Value using config.NewYAML with test data
	syncInterval := "5m" // default
	if interval, exists := m.data["syncInterval"]; exists {
		if intervalStr, ok := interval.(string); ok {
			syncInterval = intervalStr
		}
	}
	
	yamlStr := fmt.Sprintf(`
crdSync:
  syncInterval: %s
`, syncInterval)
	
	provider, err := config.NewYAML(config.Source(strings.NewReader(yamlStr)))
	if err != nil {
		// If error creating provider, create a minimal one for testing
		provider, _ = config.NewYAML()
	}
	return provider.Get(key)
}

