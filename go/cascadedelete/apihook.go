package cascadedelete

import (
	"context"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// StampOwnerRefOnCreate stamps owner as the controller ownerReference on child
// via EnsureControllerRef, so a child created through the API server already
// carries its owner ref and is never GC-eligible-but-unprotected.
//
// The caller resolves and supplies the already-loaded owner. It is best-effort
// and non-fatal: on a nil owner, an AlreadyOwnedError, or any other failure it
// logs and returns nil, because creation must not break if the owner ref cannot
// be stamped.
func StampOwnerRefOnCreate(ctx context.Context, logger *zap.Logger, scheme *runtime.Scheme, child, owner client.Object) error {
	if owner == nil {
		return nil
	}

	changed, err := EnsureControllerRef(child, owner, scheme)
	if err != nil {
		logger.Warn("BeforeCreate: failed to set ownerReference on child",
			zap.String("owner", owner.GetName()),
			zap.String("child", child.GetName()),
			zap.Error(err))
		return nil
	}
	if changed {
		logger.Info("Stamped ownerReference on child at creation",
			zap.String("owner", owner.GetName()),
			zap.String("child", child.GetName()))
	}
	return nil
}
