package common

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/dynamic"

	"github.com/michelangelo-ai/michelangelo/go/components/common/routing"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/common/routenames"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
)

// stagedEntry is the model config entry RollingRolloutActor leaves behind.
func stagedEntry() modelconfig.ModelConfigEntry {
	return modelconfig.ModelConfigEntry{
		Name:           testModelName,
		StoragePath:    testModelStoragePath,
		DeploymentName: testDeploymentName,
		Phase:          modelconfig.ModelPhaseStaged,
	}
}

// servingEntry is the entry after TrafficRoutingActor has promoted the model.
func servingEntry() modelconfig.ModelConfigEntry {
	entry := stagedEntry()
	entry.Phase = modelconfig.ModelPhaseServing
	return entry
}

func TestTrafficRoutingActor_Retrieve(t *testing.T) {
	routeName := routenames.TrafficRouteName(testISName)
	matchPath := routenames.TrafficMatchPath(testISName, testDeploymentName)
	rewritePath := routenames.TrafficRewritePath(testModelName)
	rule := routing.Rule{MatchPath: matchPath, RewritePath: rewritePath}

	tests := []struct {
		name              string
		clientErrs        clientErrors
		setupMocks        func(*rolloutMocks)
		expectedStatus    apipb.ConditionStatus
		expectedMessage   string
		expectedReasonSub string
	}{
		{
			name:              "GetDynamicClient errors",
			clientErrs:        clientErrors{getDynamicClient: errors.New("dial timeout")},
			setupMocks:        func(*rolloutMocks) {},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "dial timeout",
		},
		{
			name: "RuleExists errors",
			setupMocks: func(m *rolloutMocks) {
				m.routeManager.EXPECT().RuleExists(gomock.Any(), gomock.Any(), routeName, testNamespace, rule).
					Return(false, errors.New("api error"))
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "api error",
		},
		{
			name: "rule not present or model differs",
			setupMocks: func(m *rolloutMocks) {
				m.routeManager.EXPECT().RuleExists(gomock.Any(), gomock.Any(), routeName, testNamespace, rule).
					Return(false, nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   ReasonTrafficRouteNotReady,
			expectedReasonSub: "traffic route for deployment test-deployment is not configured for model model-v1 in cluster c1",
		},
		{
			name:       "rule present but GetClient errors",
			clientErrs: clientErrors{getClient: errors.New("auth refused")},
			setupMocks: func(m *rolloutMocks) {
				m.routeManager.EXPECT().RuleExists(gomock.Any(), gomock.Any(), routeName, testNamespace, rule).
					Return(true, nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "auth refused",
		},
		{
			name: "rule present but the model is still staged",
			setupMocks: func(m *rolloutMocks) {
				m.routeManager.EXPECT().RuleExists(gomock.Any(), gomock.Any(), routeName, testNamespace, rule).
					Return(true, nil)
				m.modelConfigProvider.EXPECT().GetModelsFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), testISName, testNamespace).
					Return([]modelconfig.ModelConfigEntry{stagedEntry()}, nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   ReasonModelNotPromoted,
			expectedReasonSub: "model model-v1 is not in the serving phase in cluster c1",
		},
		{
			name: "rule present and model serving",
			setupMocks: func(m *rolloutMocks) {
				m.routeManager.EXPECT().RuleExists(gomock.Any(), gomock.Any(), routeName, testNamespace, rule).
					Return(true, nil)
				m.modelConfigProvider.EXPECT().GetModelsFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), testISName, testNamespace).
					Return([]modelconfig.ModelConfigEntry{servingEntry()}, nil)
			},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mocks, target := newRolloutFixture(t, tt.clientErrs, true)
			tt.setupMocks(mocks)

			actor := NewTrafficRoutingActor(mocks.deps(testSettings), target)
			got, err := actor.Retrieve(context.Background(), rolloutDeployment(""), &apipb.Condition{})

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, got.Status)
			if tt.expectedMessage != "" {
				assert.Equal(t, tt.expectedMessage, got.Message)
			}
			if tt.expectedReasonSub != "" {
				assert.Contains(t, got.Reason, tt.expectedReasonSub)
			}
		})
	}
}

func TestTrafficRoutingActor_Run(t *testing.T) {
	routeName := routenames.TrafficRouteName(testISName)
	wantRule := routing.Rule{
		MatchPath:   routenames.TrafficMatchPath(testISName, testDeploymentName),
		MatchType:   routing.PathMatchPrefix,
		RewritePath: routenames.TrafficRewritePath(testModelName),
		RewriteType: routing.RewritePrefix,
		BackendName: testISName + "-inference-service",
	}
	expectEntries := func(m *rolloutMocks, entries ...modelconfig.ModelConfigEntry) {
		m.modelConfigProvider.EXPECT().GetModelsFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), testISName, testNamespace).
			Return(entries, nil)
	}
	expectPromotion := func(m *rolloutMocks) {
		m.modelConfigProvider.EXPECT().AddModelToConfig(gomock.Any(), gomock.Any(), gomock.Any(), testISName, testNamespace, servingEntry()).
			Return(nil)
	}

	tests := []struct {
		name              string
		clientErrs        clientErrors
		condition         *apipb.Condition
		setupMocks        func(*rolloutMocks)
		expectedStatus    apipb.ConditionStatus
		expectedMessage   string
		expectedReasonSub string
	}{
		{
			name:            "terminal failure from Retrieve passes through untouched",
			condition:       terminalCondition(ReasonModelLoadFailed),
			setupMocks:      func(*rolloutMocks) {},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: ReasonModelLoadFailed,
		},
		{
			name:              "GetDynamicClient errors retry",
			clientErrs:        clientErrors{getDynamicClient: errors.New("dial timeout")},
			setupMocks:        func(*rolloutMocks) {},
			expectedStatus:    apipb.CONDITION_STATUS_UNKNOWN,
			expectedMessage:   "DynamicClientUnavailable",
			expectedReasonSub: "dial timeout",
		},
		{
			name:              "GetDynamicClient errors fail once the budget is spent",
			clientErrs:        clientErrors{getDynamicClient: errors.New("dial timeout")},
			condition:         conditionWithProgress(t, startedAgo(11*time.Minute, "")),
			setupMocks:        func(*rolloutMocks) {},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   ReasonTrafficRoutingTimeout,
			expectedReasonSub: "dial timeout",
		},
		{
			name: "model entry missing means traffic is not switched",
			setupMocks: func(m *rolloutMocks) {
				expectEntries(m)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   ReasonModelEntryMissing,
			expectedReasonSub: "model model-v1 has no entry in the model config of cluster c1",
		},
		{
			name: "promotes the model and waits while a replica lacks it",
			setupMocks: func(m *rolloutMocks) {
				expectEntries(m, stagedEntry())
				expectPromotion(m)
				m.expectModelStatus(testModelName, statusOf(2, readyReplica("pod-a"), loadingReplica("pod-b")), nil)
				// AddRules must not be called: the route stays on the previous model.
			},
			expectedStatus:    apipb.CONDITION_STATUS_UNKNOWN,
			expectedMessage:   ReasonWaitingForReplicas,
			expectedReasonSub: "not routing traffic to model model-v1 in cluster c1 until every replica serves it",
		},
		{
			name: "a replica that failed the model is terminal",
			setupMocks: func(m *rolloutMocks) {
				expectEntries(m, servingEntry())
				m.expectModelStatus(testModelName, statusOf(2, readyReplica("pod-a"), failedReplica("pod-b", "OOM")), nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   ReasonModelLoadFailed,
			expectedReasonSub: "pod-b: OOM",
		},
		{
			name: "AddRules errors",
			setupMocks: func(m *rolloutMocks) {
				expectEntries(m, servingEntry())
				m.expectModelStatus(testModelName, statusOf(2, readyReplica("pod-a"), readyReplica("pod-b")), nil)
				m.routeManager.EXPECT().AddRules(gomock.Any(), gomock.Any(), routeName, testNamespace, gomock.Any()).
					Return(errors.New("update failed"))
			},
			expectedStatus:    apipb.CONDITION_STATUS_UNKNOWN,
			expectedMessage:   ReasonTrafficRouteUpsertFail,
			expectedReasonSub: "update failed",
		},
		{
			name:      "AddRules errors fail once the budget is spent",
			condition: conditionWithProgress(t, startedAgo(11*time.Minute, "")),
			setupMocks: func(m *rolloutMocks) {
				expectEntries(m, servingEntry())
				m.expectModelStatus(testModelName, statusOf(2, readyReplica("pod-a"), readyReplica("pod-b")), nil)
				m.routeManager.EXPECT().AddRules(gomock.Any(), gomock.Any(), routeName, testNamespace, gomock.Any()).
					Return(errors.New("update failed"))
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   ReasonTrafficRoutingTimeout,
			expectedReasonSub: "update failed",
		},
		{
			name: "happy path switches traffic once every replica serves the model",
			setupMocks: func(m *rolloutMocks) {
				expectEntries(m, stagedEntry())
				expectPromotion(m)
				m.expectModelStatus(testModelName, statusOf(2, readyReplica("pod-a"), readyReplica("pod-b")), nil)
				m.routeManager.EXPECT().AddRules(gomock.Any(), gomock.Any(), routeName, testNamespace, gomock.Any()).
					DoAndReturn(func(_ context.Context, _ dynamic.Interface, _, _ string, rules ...routing.Rule) error {
						assert.Equal(t, []routing.Rule{wantRule}, rules)
						return nil
					})
			},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
		},
		{
			name: "an already serving entry is not rewritten",
			setupMocks: func(m *rolloutMocks) {
				expectEntries(m, servingEntry())
				m.expectModelStatus(testModelName, statusOf(1, readyReplica("pod-a")), nil)
				m.routeManager.EXPECT().AddRules(gomock.Any(), gomock.Any(), routeName, testNamespace, gomock.Any()).Return(nil)
			},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mocks, target := newRolloutFixture(t, tt.clientErrs, true)
			tt.setupMocks(mocks)
			condition := tt.condition
			if condition == nil {
				condition = &apipb.Condition{}
			}

			actor := NewTrafficRoutingActor(mocks.deps(testSettings), target)
			got, err := actor.Run(context.Background(), rolloutDeployment(""), condition)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, got.Status)
			if tt.expectedMessage != "" {
				assert.Equal(t, tt.expectedMessage, got.Message)
			}
			if tt.expectedReasonSub != "" {
				assert.Contains(t, got.Reason, tt.expectedReasonSub)
			}
		})
	}
}

func TestTrafficRoutingActor_GetType(t *testing.T) {
	mocks, target := newRolloutFixture(t, clientErrors{}, true)
	actor := NewTrafficRoutingActor(mocks.deps(testSettings), target)
	assert.Equal(t, "TrafficRoutingConfigured-"+testCluster, actor.GetType())
}
