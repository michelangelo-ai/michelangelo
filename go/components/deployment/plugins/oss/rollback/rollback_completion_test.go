package rollback

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/michelangelo-ai/michelangelo/go/components/common/routing"
	osscommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/common/routenames"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
)

func TestRollbackCompletionActor(t *testing.T) {
	discoveryRoute := routenames.DiscoveryRouteName(testISName)
	discoveryMatch := routenames.DiscoveryMatchPath(testISName, testDeploymentName)

	t.Run("previous model keeps the discovery route", func(t *testing.T) {
		mocks, _ := newRollbackFixture(t, clientErrors{})
		actor := NewRollbackCompletionActor(mocks.params())

		got, err := actor.Retrieve(context.Background(), rollbackDeployment(testPrevious), &apipb.Condition{})
		require.NoError(t, err)
		assert.Equal(t, apipb.CONDITION_STATUS_TRUE, got.Status)

		got, err = actor.Run(context.Background(), rollbackDeployment(testPrevious), &apipb.Condition{})
		require.NoError(t, err)
		assert.Equal(t, apipb.CONDITION_STATUS_TRUE, got.Status)
	})

	t.Run("no previous model removes the discovery route", func(t *testing.T) {
		mocks, _ := newRollbackFixture(t, clientErrors{})
		mocks.routeManager.EXPECT().RuleExists(gomock.Any(), gomock.Any(), discoveryRoute, testNamespace, routing.Rule{MatchPath: discoveryMatch}).Return(true, nil)
		mocks.routeManager.EXPECT().RemoveRules(gomock.Any(), gomock.Any(), discoveryRoute, testNamespace, discoveryMatch).Return(nil)
		actor := NewRollbackCompletionActor(mocks.params())

		got, err := actor.Retrieve(context.Background(), rollbackDeployment(""), &apipb.Condition{})
		require.NoError(t, err)
		assert.Equal(t, apipb.CONDITION_STATUS_FALSE, got.Status)
		assert.Equal(t, ReasonDiscoveryRouteStillPresent, got.Message)

		got, err = actor.Run(context.Background(), rollbackDeployment(""), &apipb.Condition{})
		require.NoError(t, err)
		assert.Equal(t, apipb.CONDITION_STATUS_TRUE, got.Status)
	})

	t.Run("no previous model and no discovery route is complete", func(t *testing.T) {
		mocks, _ := newRollbackFixture(t, clientErrors{})
		mocks.routeManager.EXPECT().RuleExists(gomock.Any(), gomock.Any(), discoveryRoute, testNamespace, routing.Rule{MatchPath: discoveryMatch}).Return(false, nil)
		actor := NewRollbackCompletionActor(mocks.params())

		got, err := actor.Retrieve(context.Background(), rollbackDeployment(""), &apipb.Condition{})
		require.NoError(t, err)
		assert.Equal(t, apipb.CONDITION_STATUS_TRUE, got.Status)
	})

	t.Run("RemoveRules errors", func(t *testing.T) {
		mocks, _ := newRollbackFixture(t, clientErrors{})
		mocks.routeManager.EXPECT().RemoveRules(gomock.Any(), gomock.Any(), discoveryRoute, testNamespace, discoveryMatch).Return(errors.New("update failed"))
		actor := NewRollbackCompletionActor(mocks.params())

		got, err := actor.Run(context.Background(), rollbackDeployment(""), &apipb.Condition{})
		require.NoError(t, err)
		assert.Equal(t, apipb.CONDITION_STATUS_UNKNOWN, got.Status)
		assert.Equal(t, "DiscoveryRouteRemovalFailed", got.Message)
		assert.Contains(t, got.Reason, "update failed")
	})

	t.Run("RemoveRules errors fail once the rollback budget is spent", func(t *testing.T) {
		mocks, _ := newRollbackFixture(t, clientErrors{})
		mocks.routeManager.EXPECT().RemoveRules(gomock.Any(), gomock.Any(), discoveryRoute, testNamespace, discoveryMatch).Return(errors.New("update failed"))
		actor := NewRollbackCompletionActor(mocks.params())

		condition := conditionWithProgress(t, startedAgo(testSettings.RollbackTimeout+time.Second))
		got, err := actor.Run(context.Background(), rollbackDeployment(""), condition)
		require.NoError(t, err)
		assert.Equal(t, apipb.CONDITION_STATUS_FALSE, got.Status)
		assert.Equal(t, ReasonRollbackTimeout, got.Message)
		assert.Contains(t, got.Reason, "update failed")
	})

	t.Run("type is the terminal rollback marker", func(t *testing.T) {
		mocks, _ := newRollbackFixture(t, clientErrors{})
		assert.Equal(t, osscommon.ActorTypeRollback, NewRollbackCompletionActor(mocks.params()).GetType())
	})
}
