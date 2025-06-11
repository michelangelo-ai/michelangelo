package jobs

import (
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/ray"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/spark"
	"go.uber.org/fx"
)

// Module provides the job controllers
var Module = fx.Options(
	spark.Module,
	ray.Module,
)
