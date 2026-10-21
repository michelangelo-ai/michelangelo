package state

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/compute"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/compute/computemocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// fakeManager records the indexes and the runnables added to it.
type fakeManager struct {
	manager.Manager
	indexes   []string
	runnables []manager.Runnable
}

func (m *fakeManager) GetCache() cache.Cache { return nil }

func (m *fakeManager) GetFieldIndexer() client.FieldIndexer { return m }

func (m *fakeManager) IndexField(_ context.Context, obj client.Object, field string, _ client.IndexerFunc) error {
	m.indexes = append(m.indexes, fmt.Sprintf("%T %s", obj, field))
	return nil
}

func (m *fakeManager) Add(r manager.Runnable) error {
	m.runnables = append(m.runnables, r)
	return nil
}

// moduleOptions are the module with the dependencies the controller manager
// provides it.
func moduleOptions(t *testing.T, mgr manager.Manager, cfg maconfig.SchedulerConfig) fx.Option {
	return fx.Options(
		fx.Supply(cfg, logr.Discard()),
		fx.Provide(
			func() compute.Factory { return computemocks.NewMockFactory(gomock.NewController(t)) },
			func() manager.Manager { return mgr },
			fx.Annotate(func() types.Mapper { return newMapper(false) }, fx.ResultTags(`name:"k8sengineMapper"`)),
		),
		Module,
	)
}

func TestModule(t *testing.T) {
	tests := []struct {
		name          string
		backend       string
		wantIndexes   []string
		wantRunnables int
	}{
		{
			name:          "Kueue backend",
			backend:       kueueBackend,
			wantIndexes:   []string{"*v2.RayCluster status.assignment.cluster"},
			wantRunnables: 1,
		},
		{name: "default backend", backend: "default"},
		{name: "no backend"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := &fakeManager{}
			var (
				watches cluster.KueueWatches
				st      State
				demand  Demand
			)
			fxtest.New(t,
				moduleOptions(t, mgr, maconfig.SchedulerConfig{Backend: tt.backend}),
				fx.Populate(&watches, &st, &demand),
			).RequireStart().RequireStop()
			assert.NotNil(t, watches)
			assert.NotNil(t, st)
			assert.NotNil(t, demand)
			assert.Equal(t, tt.wantIndexes, mgr.indexes)
			require.Len(t, mgr.runnables, tt.wantRunnables)
			for _, r := range mgr.runnables {
				leaderElection, ok := r.(manager.LeaderElectionRunnable)
				require.True(t, ok)
				assert.False(t, leaderElection.NeedLeaderElection(), "every replica keeps the Kueue state")
			}
		})
	}

	t.Run("unsupported Kueue API version", func(t *testing.T) {
		cfg := maconfig.SchedulerConfig{Backend: kueueBackend, Kueue: maconfig.KueueConfig{APIVersion: "v1"}}
		app := fx.New(moduleOptions(t, &fakeManager{}, cfg), fx.NopLogger)
		require.ErrorContains(t, app.Err(), `unsupported jobs.scheduler.kueue.apiVersion "v1"`)
	})
}
