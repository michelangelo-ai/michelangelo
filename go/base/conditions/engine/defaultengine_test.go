package defaultengine

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/golang/mock/gomock"
	"go.uber.org/zap"
	ctrl "sigs.k8s.io/controller-runtime"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	mockConditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces/interfaces_mock"
	api "github.com/michelangelo-ai/michelangelo/proto/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

func TestRun(t *testing.T) {
	testCases := []struct {
		name     string
		mockFunc func(mockEngine *mockConditionInterfaces.MockPlugin[*v2.Pipeline], mockCtrl *gomock.Controller)
		expected conditionInterfaces.Result
		errMsg   string
	}{
		{
			name: "first reconcile, no existing conditions, creates default UNKNOWN condition",
			mockFunc: func(mockEngine *mockConditionInterfaces.MockPlugin[*v2.Pipeline], mockCtrl *gomock.Controller) {
				mockActor := mockConditionInterfaces.NewMockConditionActor[*v2.Pipeline](mockCtrl)

				mockEngine.EXPECT().GetActors().Return([]conditionInterfaces.ConditionActor[*v2.Pipeline]{
					mockActor,
				})
				// Return empty conditions list - simulating first reconcile
				mockEngine.EXPECT().GetConditions(gomock.Any()).Return([]*api.Condition{})
				mockEngine.EXPECT().PutCondition(gomock.Any(), gomock.Any())

				mockActor.EXPECT().GetType().Return("test")
				// Actor should receive a default UNKNOWN condition (not nil)
				mockActor.EXPECT().Retrieve(context.Background(), gomock.Any(), &api.Condition{
					Type:   "test",
					Status: api.CONDITION_STATUS_UNKNOWN,
				}).Return(&api.Condition{
					Status: api.CONDITION_STATUS_FALSE,
					Type:   "test",
				}, nil)
				mockActor.EXPECT().Run(context.Background(), gomock.Any(), gomock.Any()).Return(&api.Condition{
					Status: api.CONDITION_STATUS_UNKNOWN,
					Type:   "test",
				}, nil)
			},
			expected: conditionInterfaces.Result{
				Result:       ctrl.Result{Requeue: true, RequeueAfter: defaultInactiveRequeuePeriodInSeconds * time.Second},
				AreSatisfied: false,
				IsTerminal:   false,
			},
		},
		{
			name: "Retrieve Returns True, No Run Called",
			mockFunc: func(mockEngine *mockConditionInterfaces.MockPlugin[*v2.Pipeline], mockCtrl *gomock.Controller) {
				mockActor := mockConditionInterfaces.NewMockConditionActor[*v2.Pipeline](mockCtrl)

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
				mockActor.EXPECT().GetType().Return("test")
				mockActor.EXPECT().Retrieve(context.Background(), gomock.Any(), gomock.Any()).Return(&api.Condition{
					Status: api.CONDITION_STATUS_TRUE,
					Type:   "test",
				}, nil)
			},
			expected: conditionInterfaces.Result{
				Result:       ctrl.Result{Requeue: false, RequeueAfter: 0},
				AreSatisfied: true,
				IsTerminal:   true,
			},
		},
		{
			name: "Retrieve Returns False, Run Returns True",
			mockFunc: func(mockEngine *mockConditionInterfaces.MockPlugin[*v2.Pipeline], mockCtrl *gomock.Controller) {
				mockActor := mockConditionInterfaces.NewMockConditionActor[*v2.Pipeline](mockCtrl)

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
				mockActor.EXPECT().GetType().Return("test").Times(1)
				mockActor.EXPECT().Retrieve(context.Background(), gomock.Any(), gomock.Any()).Return(&api.Condition{
					Status: api.CONDITION_STATUS_FALSE,
					Type:   "test",
				}, nil)
				mockActor.EXPECT().Run(context.Background(), gomock.Any(), gomock.Any()).Return(&api.Condition{
					Status: api.CONDITION_STATUS_TRUE,
					Type:   "test",
				}, nil)
			},
			expected: conditionInterfaces.Result{
				Result:       ctrl.Result{Requeue: true, RequeueAfter: defaultInactiveRequeuePeriodInSeconds * time.Second},
				AreSatisfied: false,
				IsTerminal:   false,
			},
		},
		{
			name: "Retrieve Returns Unknown, Run Returns False",
			mockFunc: func(mockEngine *mockConditionInterfaces.MockPlugin[*v2.Pipeline], mockCtrl *gomock.Controller) {
				mockActor := mockConditionInterfaces.NewMockConditionActor[*v2.Pipeline](mockCtrl)

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
				mockActor.EXPECT().GetType().Return("test").Times(1)
				mockActor.EXPECT().Retrieve(context.Background(), gomock.Any(), gomock.Any()).Return(&api.Condition{
					Status: api.CONDITION_STATUS_UNKNOWN,
					Type:   "test",
				}, nil)
				mockActor.EXPECT().Run(context.Background(), gomock.Any(), gomock.Any()).Return(&api.Condition{
					Status: api.CONDITION_STATUS_FALSE,
					Type:   "test",
				}, nil)
			},
			expected: conditionInterfaces.Result{
				Result:       ctrl.Result{Requeue: false, RequeueAfter: 0},
				AreSatisfied: false,
				IsTerminal:   true,
			},
		},
		{
			name: "Retrieve and Run Both Return Unknown",
			mockFunc: func(mockEngine *mockConditionInterfaces.MockPlugin[*v2.Pipeline], mockCtrl *gomock.Controller) {
				mockActor := mockConditionInterfaces.NewMockConditionActor[*v2.Pipeline](mockCtrl)

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
				mockActor.EXPECT().GetType().Return("test").Times(1)
				mockActor.EXPECT().Retrieve(context.Background(), gomock.Any(), gomock.Any()).Return(&api.Condition{
					Status: api.CONDITION_STATUS_UNKNOWN,
					Type:   "test",
				}, nil)
				mockActor.EXPECT().Run(context.Background(), gomock.Any(), gomock.Any()).Return(&api.Condition{
					Status: api.CONDITION_STATUS_UNKNOWN,
					Type:   "test",
				}, nil)
			},
			expected: conditionInterfaces.Result{
				Result:       ctrl.Result{Requeue: true, RequeueAfter: defaultInactiveRequeuePeriodInSeconds * time.Second},
				AreSatisfied: false,
				IsTerminal:   false,
			},
		},
		{
			name: "Retrieve Returns Error",
			mockFunc: func(mockEngine *mockConditionInterfaces.MockPlugin[*v2.Pipeline], mockCtrl *gomock.Controller) {
				mockActor := mockConditionInterfaces.NewMockConditionActor[*v2.Pipeline](mockCtrl)

				mockEngine.EXPECT().GetActors().Return([]conditionInterfaces.ConditionActor[*v2.Pipeline]{
					mockActor,
				})
				mockEngine.EXPECT().GetConditions(gomock.Any()).Return([]*api.Condition{
					{
						Type:   "test",
						Status: api.CONDITION_STATUS_UNKNOWN,
					},
				})
				mockActor.EXPECT().GetType().Return("test")
				mockActor.EXPECT().Retrieve(context.Background(), gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("retrieve error"))
			},
			expected: conditionInterfaces.Result{
				Result:       ctrl.Result{Requeue: false, RequeueAfter: 0},
				AreSatisfied: false,
				IsTerminal:   true,
			},
			errMsg: "",
		},
		{
			name: "Run Returns Error",
			mockFunc: func(mockEngine *mockConditionInterfaces.MockPlugin[*v2.Pipeline], mockCtrl *gomock.Controller) {
				mockActor := mockConditionInterfaces.NewMockConditionActor[*v2.Pipeline](mockCtrl)

				mockEngine.EXPECT().GetActors().Return([]conditionInterfaces.ConditionActor[*v2.Pipeline]{
					mockActor,
				})
				mockEngine.EXPECT().GetConditions(gomock.Any()).Return([]*api.Condition{
					{
						Type:   "test",
						Status: api.CONDITION_STATUS_UNKNOWN,
					},
				})

				mockActor.EXPECT().GetType().Return("test").Times(1)
				mockActor.EXPECT().Retrieve(context.Background(), gomock.Any(), gomock.Any()).Return(&api.Condition{
					Status: api.CONDITION_STATUS_FALSE,
					Type:   "test",
				}, nil)
				mockActor.EXPECT().Run(context.Background(), gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("run error"))
			},
			expected: conditionInterfaces.Result{
				Result:       ctrl.Result{Requeue: false, RequeueAfter: 0},
				AreSatisfied: false,
				IsTerminal:   true,
			},
			errMsg: "",
		},
		{
			name: "Two Actors, First Satisfied, Second Runs and Succeeds",
			mockFunc: func(mockEngine *mockConditionInterfaces.MockPlugin[*v2.Pipeline], mockCtrl *gomock.Controller) {
				mockActor := mockConditionInterfaces.NewMockConditionActor[*v2.Pipeline](mockCtrl)
				mockActor2 := mockConditionInterfaces.NewMockConditionActor[*v2.Pipeline](mockCtrl)

				mockEngine.EXPECT().GetActors().Return([]conditionInterfaces.ConditionActor[*v2.Pipeline]{
					mockActor,
					mockActor2,
				})
				mockEngine.EXPECT().GetConditions(gomock.Any()).Return([]*api.Condition{
					{
						Type:   "actor1",
						Status: api.CONDITION_STATUS_UNKNOWN,
					},
					{
						Type:   "actor2",
						Status: api.CONDITION_STATUS_UNKNOWN,
					},
				}).Times(2)

				// First actor: Retrieve returns TRUE, so Run should not be called
				mockActor.EXPECT().GetType().Return("actor1")
				mockActor.EXPECT().Retrieve(context.Background(), gomock.Any(), gomock.Any()).Return(&api.Condition{
					Status: api.CONDITION_STATUS_TRUE,
					Type:   "actor1",
				}, nil)
				mockEngine.EXPECT().PutCondition(gomock.Any(), gomock.Any())

				// Second actor: Retrieve returns FALSE, so Run should be called
				mockActor2.EXPECT().GetType().Return("actor2").Times(1)
				mockActor2.EXPECT().Retrieve(context.Background(), gomock.Any(), gomock.Any()).Return(&api.Condition{
					Status: api.CONDITION_STATUS_FALSE,
					Type:   "actor2",
				}, nil)
				mockActor2.EXPECT().Run(context.Background(), gomock.Any(), gomock.Any()).Return(&api.Condition{
					Status: api.CONDITION_STATUS_TRUE,
					Type:   "actor2",
				}, nil)
				mockEngine.EXPECT().PutCondition(gomock.Any(), gomock.Any())
			},
			expected: conditionInterfaces.Result{
				Result:       ctrl.Result{Requeue: true, RequeueAfter: defaultInactiveRequeuePeriodInSeconds * time.Second},
				AreSatisfied: false,
				IsTerminal:   false,
			},
		},
		{
			name: "Two Actors, Both Non-Satisfied, Only First Runs",
			mockFunc: func(mockEngine *mockConditionInterfaces.MockPlugin[*v2.Pipeline], mockCtrl *gomock.Controller) {
				mockActor := mockConditionInterfaces.NewMockConditionActor[*v2.Pipeline](mockCtrl)
				mockActor2 := mockConditionInterfaces.NewMockConditionActor[*v2.Pipeline](mockCtrl)

				mockEngine.EXPECT().GetActors().Return([]conditionInterfaces.ConditionActor[*v2.Pipeline]{
					mockActor,
					mockActor2,
				})
				mockEngine.EXPECT().GetConditions(gomock.Any()).Return([]*api.Condition{
					{Type: "actor1", Status: api.CONDITION_STATUS_UNKNOWN},
					{Type: "actor2", Status: api.CONDITION_STATUS_UNKNOWN},
				}).Times(2)

				// First actor: Retrieve returns FALSE, Run should be called
				mockActor.EXPECT().GetType().Return("actor1")
				mockActor.EXPECT().Retrieve(context.Background(), gomock.Any(), gomock.Any()).Return(&api.Condition{
					Status: api.CONDITION_STATUS_FALSE,
					Type:   "actor1",
				}, nil)
				mockActor.EXPECT().Run(context.Background(), gomock.Any(), gomock.Any()).Return(&api.Condition{
					Status: api.CONDITION_STATUS_UNKNOWN,
					Type:   "actor1",
				}, nil)
				mockEngine.EXPECT().PutCondition(gomock.Any(), gomock.Any())

				// Second actor: Retrieve returns UNKNOWN, but Run should NOT be called (first actor already ran)
				mockActor2.EXPECT().GetType().Return("actor2")
				mockActor2.EXPECT().Retrieve(context.Background(), gomock.Any(), gomock.Any()).Return(&api.Condition{
					Status: api.CONDITION_STATUS_UNKNOWN,
					Type:   "actor2",
				}, nil)
				mockEngine.EXPECT().PutCondition(gomock.Any(), gomock.Any())
			},
			expected: conditionInterfaces.Result{
				Result:       ctrl.Result{Requeue: true, RequeueAfter: defaultInactiveRequeuePeriodInSeconds * time.Second},
				AreSatisfied: false,
				IsTerminal:   false,
			},
		},
		{
			name: "Two Actors, Both Satisfied",
			mockFunc: func(mockEngine *mockConditionInterfaces.MockPlugin[*v2.Pipeline], mockCtrl *gomock.Controller) {
				mockActor := mockConditionInterfaces.NewMockConditionActor[*v2.Pipeline](mockCtrl)
				mockActor2 := mockConditionInterfaces.NewMockConditionActor[*v2.Pipeline](mockCtrl)

				mockEngine.EXPECT().GetActors().Return([]conditionInterfaces.ConditionActor[*v2.Pipeline]{
					mockActor,
					mockActor2,
				})
				mockEngine.EXPECT().GetConditions(gomock.Any()).Return([]*api.Condition{
					{Type: "actor1", Status: api.CONDITION_STATUS_UNKNOWN},
					{Type: "actor2", Status: api.CONDITION_STATUS_UNKNOWN},
				}).Times(2)

				// First actor: Retrieve returns TRUE
				mockActor.EXPECT().GetType().Return("actor1")
				mockActor.EXPECT().Retrieve(context.Background(), gomock.Any(), gomock.Any()).Return(&api.Condition{
					Status: api.CONDITION_STATUS_TRUE,
					Type:   "actor1",
				}, nil)
				mockEngine.EXPECT().PutCondition(gomock.Any(), gomock.Any())

				// Second actor: Retrieve returns TRUE
				mockActor2.EXPECT().GetType().Return("actor2")
				mockActor2.EXPECT().Retrieve(context.Background(), gomock.Any(), gomock.Any()).Return(&api.Condition{
					Status: api.CONDITION_STATUS_TRUE,
					Type:   "actor2",
				}, nil)
				mockEngine.EXPECT().PutCondition(gomock.Any(), gomock.Any())
			},
			expected: conditionInterfaces.Result{
				Result:       ctrl.Result{Requeue: false, RequeueAfter: 0},
				AreSatisfied: true,
				IsTerminal:   true,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockPlugin := mockConditionInterfaces.NewMockPlugin[*v2.Pipeline](mockCtrl)
			testCase.mockFunc(mockPlugin, mockCtrl)

			engine := NewDefaultEngine[*v2.Pipeline](zap.NewNop())
			result, err := engine.Run(context.Background(), mockPlugin, &v2.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
				},
			})
			require.NoError(t, err)
			require.Equal(t, testCase.expected, result)
		})
	}
}
