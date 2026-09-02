package revision

import (
	"context"
	"fmt"

	"github.com/gogo/protobuf/proto"
	pbtypes "github.com/gogo/protobuf/types"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	"github.com/michelangelo-ai/michelangelo/go/api"
	apiutils "github.com/michelangelo-ai/michelangelo/go/api/utils"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

type revisionManager struct {
	handler api.Handler
	scheme  *runtime.Scheme
	logger  *zap.Logger
}

// NewManager creates a Manager backed by the given API handler.
// The scheme is used to derive the GVK of the BaseCR in UpsertParams.
func NewManager(handler api.Handler, scheme *runtime.Scheme, logger *zap.Logger) Manager {
	return &revisionManager{handler: handler, scheme: scheme, logger: logger}
}

func (m *revisionManager) UpsertRevision(ctx context.Context, input UpsertParams, opts UpsertOpts) (bool, error) {
	rev, err := buildRevision(input, m.scheme)
	if err != nil {
		return false, fmt.Errorf("build revision from input: %w", err)
	}

	namespace := rev.GetNamespace()
	name := rev.GetName()
	logger := m.logger.With(
		zap.String("revision_name", name),
		zap.String("namespace", namespace),
	)

	existing := rev.DeepCopy()
	err = m.handler.Get(ctx, namespace, name, &metav1.GetOptions{}, existing)
	if err != nil {
		if !apiutils.IsNotFoundError(err) {
			return false, fmt.Errorf("get existing revision: %w", err)
		}

		if opts.Immutable {
			apiutils.MarkImmutable(rev)
		}
		if createErr := m.handler.Create(ctx, rev, &metav1.CreateOptions{}); createErr != nil {
			return false, fmt.Errorf("create revision %s/%s: %w", namespace, name, createErr)
		}
		logger.Info("created revision")
		return true, nil
	}

	if apiutils.IsImmutable(existing) {
		if opts.Immutable {
			return false, nil
		}
		return false, fmt.Errorf("cannot update immutable revision %s to mutable", name)
	}

	rev.SetResourceVersion(existing.GetResourceVersion())
	if opts.Immutable {
		apiutils.MarkImmutable(rev)
	}
	if updateErr := m.handler.Update(ctx, rev, &metav1.UpdateOptions{}); updateErr != nil {
		return false, fmt.Errorf("update revision %s/%s: %w", namespace, name, updateErr)
	}
	logger.Info("updated revision")
	return true, nil
}

func buildRevision(input UpsertParams, scheme *runtime.Scheme) (*v2pb.Revision, error) {
	msg, ok := input.BaseCR.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("BaseCR %T does not implement proto.Message", input.BaseCR)
	}
	content, err := pbtypes.MarshalAny(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal BaseCR: %w", err)
	}

	gvk, err := apiutil.GVKForObject(input.BaseCR, scheme)
	if err != nil {
		return nil, fmt.Errorf("determine GVK for BaseCR %T: %w", input.BaseCR, err)
	}

	rev := &v2pb.Revision{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v2pb.GroupVersion.String(),
			Kind:       "Revision",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        input.Name,
			Namespace:   input.BaseCR.GetNamespace(),
			Labels:      input.Labels,
			Annotations: input.Annotations,
		},
		Spec: v2pb.RevisionSpec{
			BaseType: &metav1.TypeMeta{
				Kind:       gvk.Kind,
				APIVersion: gvk.GroupVersion().String(),
			},
			BaseResource: &apipb.ResourceIdentifier{
				Namespace: input.BaseCR.GetNamespace(),
				Name:      input.BaseCR.GetName(),
			},
			Content:    content,
			Owner:      &v2pb.UserInfo{Name: input.Owner},
			RevisionId: input.RevisionID,
			Source:     input.Source,
			Parent:     input.Parent,
		},
	}

	if input.GitRef != "" || input.GitBranch != "" {
		rev.Spec.GitCommit = &v2pb.CommitInfo{
			GitRef: input.GitRef,
			Branch: input.GitBranch,
		}
	}

	return rev, nil
}
