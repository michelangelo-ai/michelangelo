package intf

import (
	"context"
)

// Storage is an interface that abstracts the operations for accessing
// different storage systems. By defining this interface, we can implement
// various storage backends (e.g., S3, local filesystem, etc.) that all adhere
// to the same set of behaviors. This allows the rest of the application to
// interact with any storage system in a uniform manner.
type Storage interface {
	// Read retrieves data from the storage system using a specified path.
	//
	// Parameters:
	//   - ctx (context.Context): The context for managing deadlines, cancellations,
	//     and other request-scoped values. This allows the caller to control the lifetime
	//     of the read operation.
	//   - path (string): A string that specifies the location of the data to be read.
	//     The format of the path is dependent on the storage implementation (e.g., for S3,
	//     it might be "bucketName/objectKey").
	//
	// Returns:
	//   - any: The data retrieved from storage. The type is generic (any) to allow for
	//     flexibility in how the data is represented (e.g., it could be unmarshaled JSON,
	//     a byte slice, etc.).
	//   - error: An error value that will be non-nil if the read operation fails for any
	//     reason (e.g., if the object does not exist, network issues, permission errors, etc.).
	Read(ctx context.Context, path string) (any, error)

	// Protocol returns a string that identifies the storage protocol or type.
	//
	// This method is useful for distinguishing between different storage implementations.
	// For example, an S3 storage implementation might return "s3", whereas a local filesystem
	// implementation might return "file". This helps in selecting or logging the correct
	// storage backend in use.
	//
	// Returns:
	//   - string: A protocol identifier that represents the storage system.
	Protocol() string

	// Check the error type to see if this is not found error
	IsNotFoundError(err error) bool
}
