package apihook

import (
	"context"
	"testing"

	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
)

func TestBeforeCreate_PassesThroughToNoop(t *testing.T) {
	hook := apiHook{}
	err := hook.BeforeCreate(context.Background(), &v2.CreateModelRequest{})
	assert.NoError(t, err)
}

func TestBeforeUpdate_PassesThroughToNoop(t *testing.T) {
	hook := apiHook{}
	err := hook.BeforeUpdate(context.Background(), &v2.UpdateModelRequest{})
	assert.NoError(t, err)
}
