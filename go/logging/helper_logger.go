// Copyright (c) 2022 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package logging

import (
	"encoding/json"
	"math"
)

// truncateBody is a helper function for MarshalAndTruncateRequest
// truncate string by constant maxLogSize
func truncateBody(body string) string {
	return body[0:int(math.Min(maxLogSize, float64(len(body))))]
}

// MarshalAndTruncateRequest marshal the input message into JSON form,
// convert into string and truncated within maxLogSize
func MarshalAndTruncateRequest(request interface{}) string {
	requestBytes, _ := json.Marshal(request)
	requestJSON := string(requestBytes)
	requestJSONTruncated := truncateBody(requestJSON)

	return requestJSONTruncated
}
