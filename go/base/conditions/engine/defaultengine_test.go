package defaultengine

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/golang/mock/gomock"
	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	mockConditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces/interfaces_mock"
	api "github.com/michelangelo-ai/michelangelo/proto/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/zap"
	ctrl "sigs.k8s.io/controller-runtime"
)

func TestRun(t *testing.T) {
	testCases := []struct {
		name     string
		mockFunc func(mockEngine *mockConditionInterfaces.MockPlugin[*v2.Pipeline], mockActor *mockConditionInterfaces.MockConditionActor[*v2.Pipeline])
		expected conditionInterfaces.Result
		errMsg   string
	}{
		{
			name: "Plugin Return True",
			mockFunc: func(mockEngine *mockConditionInterfaces.MockPlugin[*v2.Pipeline], mockActor *mockConditionInterfaces.MockConditionActor[*v2.Pipeline]) {
				mockEngine.EXPECT().GetActors().Return([]conditionInterfaces.ConditionActor[*v2.Pipeline]{
					mockActor,
				})
				mockEngine.EXPECT().GetConditions(gomock.Any()).Return([]*api.Condition{
					{
						Type:   "test",
						Status: api.CONDITION_STATUS_UNKNOWN,
					},
				})
				mockEngine.EXPECT().PutCondition(gomock.Any(), gomock.Any())
				mockActor.EXPECT().Run(context.Background(), gomock.Any(), gomock.Any()).Return(&api.Condition{
					Status: api.CONDITION_STATUS_TRUE,
					Type:   "test",
				}, nil)
				mockActor.EXPECT().GetType().Return("test")
			},
			expected: conditionInterfaces.Result{
				Result:       ctrl.Result{Requeue: false, RequeueAfter: 0},
				AreSatisfied: true,
				IsTerminal:   true,
			},
		},
		{
			name: "Plugin Return False",
			mockFunc: func(mockEngine *mockConditionInterfaces.MockPlugin[*v2.Pipeline], mockActor *mockConditionInterfaces.MockConditionActor[*v2.Pipeline]) {
				mockEngine.EXPECT().GetActors().Return([]conditionInterfaces.ConditionActor[*v2.Pipeline]{
					mockActor,
				})
				mockEngine.EXPECT().GetConditions(gomock.Any()).Return([]*api.Condition{
					{
						Type:   "test",
						Status: api.CONDITION_STATUS_UNKNOWN,
					},
				})
				mockEngine.EXPECT().PutCondition(gomock.Any(), gomock.Any())
				mockActor.EXPECT().Run(context.Background(), gomock.Any(), gomock.Any()).Return(&api.Condition{
					Status: api.CONDITION_STATUS_FALSE,
					Type:   "test",
				}, nil)
				mockActor.EXPECT().GetType().Return("test")
			},
			expected: conditionInterfaces.Result{
				Result:       ctrl.Result{Requeue: false, RequeueAfter: 0},
				AreSatisfied: false,
				IsTerminal:   true,
			},
		},
		{
			name: "Plugin Return Unknown",
			mockFunc: func(mockEngine *mockConditionInterfaces.MockPlugin[*v2.Pipeline], mockActor *mockConditionInterfaces.MockConditionActor[*v2.Pipeline]) {
				mockEngine.EXPECT().GetActors().Return([]conditionInterfaces.ConditionActor[*v2.Pipeline]{
					mockActor,
				})
				mockEngine.EXPECT().GetConditions(gomock.Any()).Return([]*api.Condition{
					{
						Type:   "test",
						Status: api.CONDITION_STATUS_UNKNOWN,
					},
				})
				mockEngine.EXPECT().PutCondition(gomock.Any(), gomock.Any())
				mockActor.EXPECT().Run(context.Background(), gomock.Any(), gomock.Any()).Return(&api.Condition{
					Status: api.CONDITION_STATUS_UNKNOWN,
					Type:   "test",
				}, nil)
				mockActor.EXPECT().GetType().Return("test")
			},
			expected: conditionInterfaces.Result{
				Result:       ctrl.Result{Requeue: true, RequeueAfter: defaultInactiveRequeuePeriodInSeconds * time.Second},
				AreSatisfied: false,
				IsTerminal:   false,
			},
		},
		{
			name: "Plugin Returns Error",
			mockFunc: func(mockEngine *mockConditionInterfaces.MockPlugin[*v2.Pipeline], mockActor *mockConditionInterfaces.MockConditionActor[*v2.Pipeline]) {
				mockEngine.EXPECT().GetActors().Return([]conditionInterfaces.ConditionActor[*v2.Pipeline]{
					mockActor,
				})
				mockEngine.EXPECT().GetConditions(gomock.Any()).Return([]*api.Condition{
					{
						Type:   "test",
						Status: api.CONDITION_STATUS_UNKNOWN,
					},
				})
				mockActor.EXPECT().Run(context.Background(), gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("test error"))
				mockActor.EXPECT().GetType().Return("test")
			},
			expected: conditionInterfaces.Result{
				Result:       ctrl.Result{Requeue: true, RequeueAfter: defaultInactiveRequeuePeriodInSeconds * time.Second},
				AreSatisfied: false,
				IsTerminal:   false,
			},
			errMsg: "test error",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockPlugin := mockConditionInterfaces.NewMockPlugin[*v2.Pipeline](mockCtrl)
			mockActor := mockConditionInterfaces.NewMockConditionActor[*v2.Pipeline](mockCtrl)
			testCase.mockFunc(mockPlugin, mockActor)

			engine := NewDefaultEngine[*v2.Pipeline](zap.NewNop())
			result, err := engine.Run(context.Background(), mockPlugin, &v2.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
			})
			if testCase.errMsg != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), testCase.errMsg)
			} else {
				require.NoError(t, err)
				require.Equal(t, testCase.expected, result)
			}
		})
	}
}
