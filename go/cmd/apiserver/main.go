package main

import (
	"context"

	apihandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/auth"
	"github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/base/zapfx"
	"github.com/michelangelo-ai/michelangelo/go/logging"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/uber-go/tally"
	"go.uber.org/fx"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
)

func main() {
	fx.New(
		opts(),
	).Run()
}

func opts() fx.Option {
	return fx.Options(
		env.Module,
		config.Module,
		zapfx.Module,
		apihandler.APIServerModule,
		fx.Provide(getDummyAuth),
		fx.Provide(getDummyAuditLog),
		fx.Provide(getTallyScope),
		fx.Provide(getK8sRestConfig),
		fx.Provide(getYARPCConfig),
		fx.Provide(getMetadataStorageConfig),
		fx.Provide(provideDispatcher),
		fx.Provide(getScheme),
		v2pb.ProjectSvcModule,
		v2pb.RayClusterSvcModule,
		v2pb.RayJobSvcModule,
		v2pb.SparkJobSvcModule,
		fx.Invoke(registerProcedures),
		fx.Invoke(startYARPCServer),
	)
}

func getDummyAuth() auth.Auth {
	return auth.DummyAuth{}
}

func getTallyScope() (tally.Scope, error) {
	s, _ := tally.NewRootScopeWithDefaultInterval(tally.ScopeOptions{
		Prefix: "michelangelo-apiserver",
	})
	return s, nil
}

type DummyAuditLog struct{}

func (d *DummyAuditLog) Emit(_ context.Context, _ *logging.AuditLogEvent) {
}

func getDummyAuditLog() logging.AuditLog {
	return &DummyAuditLog{}
}

func getScheme() *runtime.Scheme {
	return scheme.Scheme
}
