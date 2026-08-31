package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr/funcr"
	"github.com/golang/mock/gomock"
	"github.com/michelangelo-ai/michelangelo/go/storage/storagemocks"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestHandleDeleteBlobStorageErrorLogging verifies that handleDelete logs a
// blob-storage deletion failure only when the deletion actually fails: a
// successful delete must not emit the "Failed to delete object in blob
// storage" error line.
func TestHandleDeleteBlobStorageErrorLogging(t *testing.T) {
	typeMeta := &metav1.TypeMeta{Kind: "RayJob"}

	testCases := []struct {
		name          string
		blobDeleteErr error
		wantErrorLogs int
	}{
		{
			name:          "no error log when blob delete succeeds",
			blobDeleteErr: nil,
			wantErrorLogs: 0,
		},
		{
			name:          "one error log when blob delete fails",
			blobDeleteErr: errors.New("blob backend unavailable"),
			wantErrorLogs: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			metadataStorage := storagemocks.NewMockMetadataStorage(ctrl)
			blobStorage := storagemocks.NewMockBlobStorage(ctrl)

			obj := &v2pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "job01",
					UID:       "uid-01",
				},
			}

			blobStorage.EXPECT().IsObjectInteresting(gomock.Any()).Return(true)
			metadataStorage.EXPECT().GetByID(gomock.Any(), "uid-01", gomock.Any()).Return(nil)
			metadataStorage.EXPECT().Delete(gomock.Any(), typeMeta, "default", "job01").Return(nil)
			blobStorage.EXPECT().DeleteFromBlobStorage(gomock.Any(), gomock.Any()).Return(tc.blobDeleteErr)

			var logLines []string
			log := funcr.New(func(prefix, args string) {
				logLines = append(logLines, args)
			}, funcr.Options{})

			err := handleDelete(context.Background(), log, typeMeta, obj, metadataStorage, NewBlobHandler(blobStorage))

			assert.NoError(t, err)
			assert.Len(t, logLines, tc.wantErrorLogs)
		})
	}
}
