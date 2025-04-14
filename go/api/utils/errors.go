package utils

import (
	"go.uber.org/yarpc/encoding/protobuf"
	"go.uber.org/yarpc/yarpcerrors"
)

// NotFound error means some requested entity was not found. This is considered as a client error.
func NotFound(message string) error {
	return protobuf.NewError(yarpcerrors.CodeNotFound, message)
}

// InvalidArgument error means the client specified an invalid argument. This is considered as a client error.
func InvalidArgument(message string) error {
	return protobuf.NewError(yarpcerrors.CodeInvalidArgument, message)
}

// Unimplemented means the operation is not implemented or is not supported/enabled in this service.
// This is considered as a client error.
func Unimplemented(message string) error {
	return protobuf.NewError(yarpcerrors.CodeUnimplemented, message)
}

// GetCode returns error code
func GetCode(err error) string {
	return yarpcerrors.FromError(err).Code().String()
}
