package utils

import (
	"context"
	"testing"

	"github.com/michelangelo-ai/michelangelo/go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"go.uber.org/yarpc/api/encoding"
	"go.uber.org/yarpc/api/transport"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestTypeMetaConversion(t *testing.T) {
	err := v2pb.AddToScheme(scheme.Scheme)
	assert.NoError(t, err)

	project := &v2pb.Project{}
	projectList := &v2pb.ProjectList{}

	objTypeMeta, err := GetObjectTypeMetafromObject(project, scheme.Scheme)
	assert.Nil(t, err)
	assert.Equal(t, "Project", objTypeMeta.Kind)

	_, err = GetObjectTypeMetafromObject(nil, scheme.Scheme)
	assert.Equal(t, codes.InvalidArgument, status.Convert(err).Code())

	derivedObjTypeMeta, err := GetObjectTypeMetaFromList(projectList, scheme.Scheme)
	assert.Nil(t, err)
	assert.Equal(t, objTypeMeta, derivedObjTypeMeta)

	_, err = GetObjectTypeMetaFromList(nil, scheme.Scheme)
	assert.Equal(t, codes.InvalidArgument, status.Convert(err).Code())

	_, err = GetObjectTypeMetaFromList(project, scheme.Scheme)
	assert.Equal(t, codes.InvalidArgument, status.Convert(err).Code())
}

func TestAnnotations(t *testing.T) {
	assert.Equal(t, false, IsDeleting(nil))

	project := &v2pb.Project{}
	assert.Equal(t, false, IsImmutable(project))

	annotations := make(map[string]string)
	project.SetAnnotations(annotations)

	annotations[api.DeletingAnnotation] = "true"
	annotations[api.ImmutableAnnotation] = "true"
	assert.Equal(t, true, IsDeleting(project))
	assert.Equal(t, true, IsImmutable(project))

	annotations[api.DeletingAnnotation] = "false"
	annotations[api.ImmutableAnnotation] = "false"
	assert.Equal(t, false, IsDeleting(project))
	assert.Equal(t, false, IsImmutable(project))

	MarkImmutable(project)
	assert.Equal(t, true, IsImmutable(project))

	// create annotations and mark immutable
	rayJob := &v2pb.RayJob{}
	MarkImmutable(rayJob)
	assert.Equal(t, true, IsImmutable(rayJob))

}

func TestNameConversion(t *testing.T) {
	assert.Equal(t, "model", ToSnakeCase("Model"))
	assert.Equal(t, "test_indexing", ToSnakeCase("TestIndexing"))
	assert.Equal(t, "pipeline_run", ToSnakeCase("pipelineRun"))
}

func TestGetHeaders(t *testing.T) {
	ctx := context.Background()
	headers := GetHeaders(ctx)
	assert.Equal(t, map[string]string{}, headers)

	ctx = setHeadersOnCtx(ctx, t, map[string]string{"email": "test@uber.com", "test": "12345678"})
	resultHeaders := GetHeaders(ctx)
	assert.Equal(t, "test@uber.com", resultHeaders["email"])
	assert.Equal(t, "12345678", resultHeaders["test"])
}

func setHeadersOnCtx(ctx context.Context, t *testing.T, h map[string]string) context.Context {
	ctx, inbound := encoding.NewInboundCall(ctx)
	headers := transport.NewHeaders()
	for k, v := range h {
		headers = headers.With(k, v)
	}
	req := &transport.Request{Headers: headers}
	err := inbound.ReadFromRequest(req)
	assert.NoError(t, err)
	return ctx
}

func CheckGrpcStatusCode(t *testing.T, expectedCode codes.Code, err error) {
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, expectedCode, grpcStatus.Code())
}
