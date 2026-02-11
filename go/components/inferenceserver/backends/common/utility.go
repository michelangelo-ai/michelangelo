package common

import (
	"fmt"
)

// GenerateInferenceServiceName generates the name of the kubernetes service for an inference server.
// This is used to identify the inference server service in the target cluster.
func GenerateInferenceServiceName(inferenceServerName string) string {
	return fmt.Sprintf("%s-inference-service", inferenceServerName)
}
