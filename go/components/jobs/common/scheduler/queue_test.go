package scheduler

import (
	"context"
	"testing"
	"time"

	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"mock/context/contextmock"
)

func TestQueue(t *testing.T) {
	tt := []struct {
		haveItems    []types.SchedulableJob
		setupContext func(t *testing.T) context.Context
		wantLength   int
		wantErr      error
	}{
		{
			haveItems: []types.SchedulableJob{},
			setupContext: func(t *testing.T) context.Context {
				return context.Background()
			},
			wantLength: 0,
			wantErr:    nil,
		},
		{
			haveItems: []types.SchedulableJob{
				types.NewSchedulableJob(types.SchedulableJobParams{
					Name:       "job-1",
					Namespace:  "test-ns",
					Generation: 1,
					JobType:    types.RayJob,
				}),
				types.NewSchedulableJob(types.SchedulableJobParams{
					Name:       "job-2",
					Namespace:  "test-ns",
					Generation: 1,
					JobType:    types.SparkJob,
				}),
			},
			setupContext: func(t *testing.T) context.Context {
				return context.Background()
			},
			wantLength: 2,
			wantErr:    nil,
		},
	}

	q := New().Queue
	for _, test := range tt {
		ctx := test.setupContext(t)
		for _, j := range test.haveItems {
			err := q.Add(context.Background(), j)
			require.NoError(t, err)
		}

		qLen := q.Length()
		require.Equal(t, test.wantLength, qLen)

		for _, wj := range test.haveItems {
			item, err := q.Get(ctx)
			require.NoError(t, err)
			require.Equal(t, wj.GetName(), item.GetName())
		}
	}
}

func TestQueueError(t *testing.T) {
	t.Run("push error", func(t *testing.T) {
		q := channelQueue{
			q: make(chan types.SchedulableJob, 0),
		}
		ctrl := gomock.NewController(t)
		mCtx := contextmock.NewMockContext(ctrl)

		ch := make(chan struct{})
		close(ch)

		mCtx.EXPECT().Done().Return(ch).Times(1)
		mCtx.EXPECT().Err().Return(context.Canceled)

		err := q.Add(mCtx, types.NewSchedulableJob(types.SchedulableJobParams{}))
		require.Error(t, err)
	})

	t.Run("pop error", func(t *testing.T) {
		q := channelQueue{
			q: make(chan types.SchedulableJob, 0),
		}
		ctrl := gomock.NewController(t)
		mCtx := contextmock.NewMockContext(ctrl)

		ch := make(chan struct{})
		close(ch)

		mCtx.EXPECT().Done().Return(ch).Times(1)
		mCtx.EXPECT().Err().Return(context.Canceled)

		_, err := q.Get(mCtx)
		require.Error(t, err)
	})
}

func TestQueueDuplicates(t *testing.T) {
	ops := []struct {
		job               types.SchedulableJob
		expectedPushError error
		msg               string
	}{
		{
			job: types.NewSchedulableJob(types.SchedulableJobParams{
				Name:       "job-1",
				Namespace:  "test-ns",
				Generation: 1,
				JobType:    types.RayJob,
			}),
			expectedPushError: nil,
			msg:               "new job gets pushed in",
		},
		{
			job: types.NewSchedulableJob(types.SchedulableJobParams{
				Name:       "job-2",
				Namespace:  "test-ns",
				Generation: 1,
				JobType:    types.RayJob,
			}),
			expectedPushError: nil,
			msg:               "second job gets pushed in",
		},
		{
			job: types.NewSchedulableJob(types.SchedulableJobParams{
				Name:       "job-1",
				Namespace:  "test-ns",
				Generation: 1,
				JobType:    types.SparkJob,
			}),
			expectedPushError: types.ErrJobAlreadyExists,
			msg:               "duplicate error - job type doesn't matter",
		},
		{
			job: types.NewSchedulableJob(types.SchedulableJobParams{
				Name:       "job-3",
				Namespace:  "test-ns",
				Generation: 1,
				JobType:    types.RayJob,
			}),
			expectedPushError: context.DeadlineExceeded,
			msg:               "queue full - context deadline exceed error",
		},
	}

	q := &channelQueue{
		q:          make(chan types.SchedulableJob, 2),
		dirty:      make(map[string]struct{}),
		processing: make(map[string]struct{}),
	}
	expectedEntries := make(map[string]struct{})
	for _, op := range ops {
		t.Run(op.msg, func(t *testing.T) {
			func() {
				ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
				defer cancel()
				err := q.Add(ctx, op.job)
				require.Equal(t, op.expectedPushError, err)

				if err == nil {
					expectedEntries[q.getKey(op.job)] = struct{}{}
				}
			}()
		})
	}

	require.Equal(t, expectedEntries, q.dirty)
}
