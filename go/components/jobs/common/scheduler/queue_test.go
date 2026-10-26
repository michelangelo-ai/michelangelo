package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/michelangelo-ai/michelangelo/go/base/typedefs/context/contextmocks"

	"github.com/golang/mock/gomock"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	"github.com/stretchr/testify/require"
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
		mCtx := contextmocks.NewMockContext(ctrl)

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
		mCtx := contextmocks.NewMockContext(ctrl)

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
				JobType:    types.RayJob,
			}),
			expectedPushError: types.ErrJobAlreadyExists,
			msg:               "duplicate error - same job type, namespace, name and generation",
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

// TestQueueGetKey covers the two ways the dedupe key aliased distinct jobs before
// it carried the job type and separators, the pairs a differently built
// key could still alias, and that a genuine duplicate is still rejected.
func TestQueueGetKey(t *testing.T) {
	t.Run("job type disambiguates", func(t *testing.T) {
		q := New().Queue
		ctx := context.Background()

		require.NoError(t, q.Add(ctx, types.NewSchedulableJob(types.SchedulableJobParams{
			Name:       "job-1",
			Namespace:  "test-ns",
			Generation: 1,
			JobType:    types.SparkJob,
		})))
		require.NoError(t, q.Add(ctx, types.NewSchedulableJob(types.SchedulableJobParams{
			Name:       "job-1",
			Namespace:  "test-ns",
			Generation: 1,
			JobType:    types.RayCluster,
		})))

		require.Equal(t, 2, q.Length())
	})

	t.Run("the namespace and name boundary is unambiguous", func(t *testing.T) {
		q := New().Queue
		ctx := context.Background()

		require.NoError(t, q.Add(ctx, types.NewSchedulableJob(types.SchedulableJobParams{
			Name:       "c",
			Namespace:  "ab",
			Generation: 1,
			JobType:    types.RayCluster,
		})))
		require.NoError(t, q.Add(ctx, types.NewSchedulableJob(types.SchedulableJobParams{
			Name:       "bc",
			Namespace:  "a",
			Generation: 1,
			JobType:    types.RayCluster,
		})))

		require.Equal(t, 2, q.Length())
	})

	t.Run("jobs that differ only at a boundary the key could blur get distinct entries", func(t *testing.T) {
		job := func(jobType types.JobType, name string, generation int64) types.SchedulableJob {
			return types.NewSchedulableJob(types.SchedulableJobParams{
				Name:       name,
				Namespace:  "test-ns",
				Generation: generation,
				JobType:    jobType,
			})
		}

		tests := []struct {
			name string
			a, b types.SchedulableJob
		}{
			{
				// JobType.ToString() is "" for RayCluster and for every type it does
				// not know, so a key built from it would alias them.
				name: "RayCluster and an unknown job type",
				a:    job(types.RayCluster, "job-1", 1),
				b:    job(types.JobType(99), "job-1", 1),
			},
			{
				name: "RayCluster and an unset job type",
				a:    job(types.RayCluster, "job-1", 1),
				b:    job(types.JobType(0), "job-1", 1),
			},
			{
				// Without a separator between them, "foo" and 11 read the same as
				// "foo1" and 1.
				name: "the name and generation boundary",
				a:    job(types.RayCluster, "foo", 11),
				b:    job(types.RayCluster, "foo1", 1),
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				q := New().Queue
				ctx := context.Background()

				require.NoError(t, q.Add(ctx, tt.a))
				require.NoError(t, q.Add(ctx, tt.b))

				require.Equal(t, 2, q.Length())
			})
		}
	})

	t.Run("an identical job is still a duplicate", func(t *testing.T) {
		q := New().Queue
		ctx := context.Background()

		job := types.NewSchedulableJob(types.SchedulableJobParams{
			Name:       "job-1",
			Namespace:  "test-ns",
			Generation: 1,
			JobType:    types.SparkJob,
		})

		require.NoError(t, q.Add(ctx, job))
		require.ErrorIs(t, q.Add(ctx, job), types.ErrJobAlreadyExists)
		require.Equal(t, 1, q.Length())
	})
}
