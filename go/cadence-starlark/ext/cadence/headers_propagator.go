package cadence

import (
	"context"
	commonpb "go.temporal.io/api/common/v1"
	"go.temporal.io/sdk/workflow"
)

var (
	ctxKeyHeaders ctxKey = 1
)

type ctxKey int

type hasValue interface {
	Value(key interface{}) interface{}
}

type contextPropagatorHeaders struct{}

// ContextPropagatorHeaders is an instance of contextPropagatorHeaders
var ContextPropagatorHeaders workflow.ContextPropagator = &contextPropagatorHeaders{}

// Inject extracts specific values from the context and writes them into the workflow headers.
func (r *contextPropagatorHeaders) Inject(ctx context.Context, writer workflow.HeaderWriter) error {
	return inject(ctx, writer)
}

// Extract retrieves specific values from the workflow headers and adds them to the context.
func (r *contextPropagatorHeaders) Extract(ctx context.Context, reader workflow.HeaderReader) (context.Context, error) {
	if h, err := readHeaders(reader); err != nil {
		return nil, err
	} else {
		return context.WithValue(ctx, ctxKeyHeaders, h), nil
	}
}

// InjectFromWorkflow extracts specific values from the workflow context and writes them into the workflow headers.
func (r *contextPropagatorHeaders) InjectFromWorkflow(ctx workflow.Context, writer workflow.HeaderWriter) error {
	return inject(ctx, writer)
}

// ExtractToWorkflow retrieves specific values from the workflow headers and adds them to the workflow context.
func (r *contextPropagatorHeaders) ExtractToWorkflow(ctx workflow.Context, reader workflow.HeaderReader) (workflow.Context, error) {
	if h, err := readHeaders(reader); err != nil {
		return nil, err
	} else {
		return workflow.WithValue(ctx, ctxKeyHeaders, h), nil
	}
}

// inject writes headers from the context into the workflow headers.
func inject(ctx hasValue, writer workflow.HeaderWriter) error {
	headers, ok := ctx.Value(ctxKeyHeaders).(map[string][]byte)
	if !ok {
		return nil // No headers to inject
	}
	for k, v := range headers {
		// Convert []byte to *commonpb.Payload
		payload := &commonpb.Payload{Data: v}
		writer.Set(k, payload)
	}
	return nil
}
func readHeaders(reader workflow.HeaderReader) (map[string][]byte, error) {
	headers := make(map[string][]byte)
	err := reader.ForEachKey(func(key string, payload *commonpb.Payload) error {
		headers[key] = payload.GetData()
		return nil
	})
	return headers, err
}
