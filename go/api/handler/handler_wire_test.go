package handler

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/go-logr/zapr"
	"github.com/golang/mock/gomock"
	authapi "github.com/michelangelo-ai/michelangelo/go/auth"
	"github.com/michelangelo-ai/michelangelo/go/logging"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	"github.com/michelangelo-ai/michelangelo/go/storage/storagemocks"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/yarpc"
	"go.uber.org/yarpc/transport/grpc"
	"go.uber.org/yarpc/yarpcerrors"
	"go.uber.org/zap"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const wireServiceName = "ma-apiserver"

// newRayJobWireClient serves the generated RayJobService handler, backed by a real apiHandler, on a local gRPC
// listener and returns a client for it. Errors cross the same YARPC gRPC transport as in the apiserver, so the code
// a test reads is the code a client such as yab receives.
func newRayJobWireClient(t *testing.T, metadataStorage storage.MetadataStorage) v2pb.RayJobServiceYARPCClient {
	t.Helper()

	require.NoError(t, v2pb.AddToScheme(scheme.Scheme))
	// The fake client has no REST mapper by default, so the handler could not tell that RayJob is namespaced and would
	// skip the namespace checks. A real cluster reports the scope, so the mapper declares it.
	restMapper := meta.NewDefaultRESTMapper(nil)
	restMapper.Add(v2pb.GroupVersion.WithKind("RayJob"), meta.RESTScopeNamespace)
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithRESTMapper(restMapper).
		WithLists(initLists...).
		Build()
	h := &apiHandler{
		conf:              storage.MetadataStorageConfig{EnableMetadataStorage: metadataStorage != nil},
		logger:            zapr.NewLogger(zap.NewNop()),
		metrics:           tally.NoopScope,
		k8sHandler:        NewK8sHandler(k8sClient),
		metadataHandler:   NewMetadataHandler(metadataStorage, nil, zapr.NewLogger(zap.NewNop())),
		blobHandler:       NewBlobHandler(nil),
		validationHandler: NewValidationHandler(),
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	serverTransport := grpc.NewTransport()
	server := yarpc.NewDispatcher(yarpc.Config{
		Name:     wireServiceName,
		Inbounds: yarpc.Inbounds{serverTransport.NewInbound(listener)},
	})
	server.Register(v2pb.BuildRayJobServiceYARPCProcedures(v2pb.NewRayJobServiceHandler(
		v2pb.FxRayJobServiceHandlerParams{
			Handler:      h,
			MetricsScope: tally.NoopScope,
			Auth:         authapi.DummyAuth{},
			AuditLog:     &logging.DummyAuditLog{},
		})))
	require.NoError(t, server.Start())
	t.Cleanup(func() { assert.NoError(t, server.Stop()) })

	clientTransport := grpc.NewTransport()
	client := yarpc.NewDispatcher(yarpc.Config{
		Name: "wire-test-client",
		Outbounds: yarpc.Outbounds{
			wireServiceName: {Unary: clientTransport.NewSingleOutbound(listener.Addr().String())},
		},
	})
	require.NoError(t, client.Start())
	t.Cleanup(func() { assert.NoError(t, client.Stop()) })

	return v2pb.NewRayJobServiceYARPCClient(client.ClientConfig(wireServiceName))
}

// wireCode is the status code a client receives: OK for a nil error, the YARPC code otherwise.
func wireCode(err error) yarpcerrors.Code {
	if err == nil {
		return yarpcerrors.CodeOK
	}
	return yarpcerrors.FromError(err).Code()
}

// wireCase is one request of the dev-10 manual test: a request shape and the code the client must see.
type wireCase struct {
	name      string
	namespace string
	objName   string
	wantCode  yarpcerrors.Code
}

// keyCases are the dev-10 requests that differ only by namespace and name. The seeded objects (see setupK8s) are
// default/job01 and default/job02.
func keyCases(existing string) []wireCase {
	return []wireCase{
		{name: "empty name is InvalidArgument", namespace: "default", objName: "", wantCode: yarpcerrors.CodeInvalidArgument},
		{name: "name with slash is InvalidArgument", namespace: "default", objName: "a/b", wantCode: yarpcerrors.CodeInvalidArgument},
		{name: "name dot dot is InvalidArgument", namespace: "default", objName: "..", wantCode: yarpcerrors.CodeInvalidArgument},
		{name: "name with percent is InvalidArgument", namespace: "default", objName: "a%b", wantCode: yarpcerrors.CodeInvalidArgument},
		{name: "empty namespace is InvalidArgument", namespace: "", objName: existing, wantCode: yarpcerrors.CodeInvalidArgument},
		{name: "namespace with slash is InvalidArgument", namespace: "a/b", objName: existing, wantCode: yarpcerrors.CodeInvalidArgument},
		{name: "namespace dot dot is InvalidArgument", namespace: "..", objName: existing, wantCode: yarpcerrors.CodeInvalidArgument},
	}
}

func wireCtx(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestGetRayJobReturnsExpectedCodeOverTheWire(t *testing.T) {
	tests := append(keyCases("job01"),
		wireCase{name: "well-formed missing object is NotFound", namespace: "default", objName: "no-such-job", wantCode: yarpcerrors.CodeNotFound},
		wireCase{name: "existing object is OK", namespace: "default", objName: "job01", wantCode: yarpcerrors.CodeOK},
	)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newRayJobWireClient(t, nil)
			resp, err := client.GetRayJob(wireCtx(t), &v2pb.GetRayJobRequest{Namespace: tt.namespace, Name: tt.objName})
			assert.Equal(t, tt.wantCode, wireCode(err), "err: %v", err)
			if tt.wantCode == yarpcerrors.CodeOK {
				assert.Equal(t, tt.objName, resp.GetRayJob().GetName())
			}
		})
	}
}

func TestCreateRayJobReturnsExpectedCodeOverTheWire(t *testing.T) {
	tests := append(keyCases("job-new"),
		wireCase{name: "new object is OK", namespace: "default", objName: "job-new", wantCode: yarpcerrors.CodeOK},
		wireCase{name: "existing object is AlreadyExists", namespace: "default", objName: "job01", wantCode: yarpcerrors.CodeAlreadyExists},
	)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newRayJobWireClient(t, nil)
			_, err := client.CreateRayJob(wireCtx(t), &v2pb.CreateRayJobRequest{RayJob: rayJob(tt.namespace, tt.objName)})
			assert.Equal(t, tt.wantCode, wireCode(err), "err: %v", err)
		})
	}
}

// With metadata storage enabled, Create looks the object up in metadata storage before it reaches k8s. A bad key must
// be rejected before that lookup, so the storage mock expects no call for the bad-key cases.
func TestCreateRayJobWithMetadataStorageReturnsExpectedCodeOverTheWire(t *testing.T) {
	notFound := apiErrors.NewNotFound(schema.GroupResource{Resource: "rayjobs"}, "job-new")
	tests := append(keyCases("job-new"),
		wireCase{name: "new object is OK", namespace: "default", objName: "job-new", wantCode: yarpcerrors.CodeOK},
	)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			metadataStorage := storagemocks.NewMockMetadataStorage(ctrl)
			if tt.wantCode == yarpcerrors.CodeOK {
				metadataStorage.EXPECT().GetByName(gomock.Any(), tt.namespace, tt.objName, gomock.Any()).Return(notFound)
			}
			client := newRayJobWireClient(t, metadataStorage)
			_, err := client.CreateRayJob(wireCtx(t), &v2pb.CreateRayJobRequest{RayJob: rayJob(tt.namespace, tt.objName)})
			assert.Equal(t, tt.wantCode, wireCode(err), "err: %v", err)
		})
	}
}

func TestUpdateRayJobReturnsExpectedCodeOverTheWire(t *testing.T) {
	tests := append(keyCases("job01"),
		wireCase{name: "well-formed missing object is NotFound", namespace: "default", objName: "no-such-job", wantCode: yarpcerrors.CodeNotFound},
		wireCase{name: "existing object is OK", namespace: "default", objName: "job01", wantCode: yarpcerrors.CodeOK},
	)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newRayJobWireClient(t, nil)
			job := rayJob(tt.namespace, tt.objName)
			if tt.wantCode == yarpcerrors.CodeOK {
				// A real client updates the object it just read, which carries the current resourceVersion.
				got, err := client.GetRayJob(wireCtx(t), &v2pb.GetRayJobRequest{Namespace: tt.namespace, Name: tt.objName})
				require.NoError(t, err)
				job = got.RayJob
			}
			_, err := client.UpdateRayJob(wireCtx(t), &v2pb.UpdateRayJobRequest{RayJob: job})
			assert.Equal(t, tt.wantCode, wireCode(err), "err: %v", err)
		})
	}
}

func TestDeleteRayJobReturnsExpectedCodeOverTheWire(t *testing.T) {
	tests := append(keyCases("job02"),
		wireCase{name: "well-formed missing object is NotFound", namespace: "default", objName: "no-such-job", wantCode: yarpcerrors.CodeNotFound},
		wireCase{name: "existing object is OK", namespace: "default", objName: "job02", wantCode: yarpcerrors.CodeOK},
	)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newRayJobWireClient(t, nil)
			_, err := client.DeleteRayJob(wireCtx(t), &v2pb.DeleteRayJobRequest{Namespace: tt.namespace, Name: tt.objName})
			assert.Equal(t, tt.wantCode, wireCode(err), "err: %v", err)
		})
	}
}

// With metadata storage enabled, Delete branches to an asynchronous path that reads the object first. A bad key must
// be rejected before that branch, so the storage mock (which fails on any call) is never reached.
func TestDeleteRayJobWithMetadataStorageRejectsBadKeyOverTheWire(t *testing.T) {
	for _, tt := range keyCases("job02") {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := newRayJobWireClient(t, storagemocks.NewMockMetadataStorage(ctrl))
			_, err := client.DeleteRayJob(wireCtx(t), &v2pb.DeleteRayJobRequest{Namespace: tt.namespace, Name: tt.objName})
			assert.Equal(t, tt.wantCode, wireCode(err), "err: %v", err)
		})
	}
}

func TestListRayJobReturnsExpectedCodeOverTheWire(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		wantCode  yarpcerrors.Code
		wantItems int
	}{
		{name: "namespace with objects is OK", namespace: "default", wantCode: yarpcerrors.CodeOK, wantItems: 2},
		{name: "namespace without objects is OK and empty", namespace: "empty-ns", wantCode: yarpcerrors.CodeOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newRayJobWireClient(t, nil)
			resp, err := client.ListRayJob(wireCtx(t), &v2pb.ListRayJobRequest{Namespace: tt.namespace})
			assert.Equal(t, tt.wantCode, wireCode(err), "err: %v", err)
			if err == nil {
				assert.Len(t, resp.GetRayJobList().GetItems(), tt.wantItems)
			}
		})
	}
}
