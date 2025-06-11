package client

import (
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/client/uke"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/secrets"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/utils"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/compute"
	"go.uber.org/fx"
)

// Module provides client for jobs
// related operations
var Module = fx.Options(
	fx.Provide(NewClient),
	fx.Provide(uke.NewUkeMapper),
	fx.Provide(NewHelper),
	fx.Provide(uke.NewSparkClient),
	fx.Provide(utils.NewMTLSHandler),
	compute.Module,
	secrets.Module,
)
