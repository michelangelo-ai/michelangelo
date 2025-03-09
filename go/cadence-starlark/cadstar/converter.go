package cadstar

import (
	"bufio"
	"bytes"
	"encoding/json"
	jsoniter "github.com/json-iterator/go"
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/ext"
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/star"
	"go.starlark.net/starlark"
	commonpb "go.temporal.io/api/common/v1"
	"go.temporal.io/sdk/converter"
	"go.uber.org/zap"
	"io"
)

const delimiter byte = '\n'

// DataConverter is a Temporal DataConverter that supports Starlark types.
// Enables passing Starlark values between Temporal workflows and activities.
type DataConverter struct {
	Logger *zap.Logger
}

var _ converter.DataConverter = (*DataConverter)(nil)

// ToStrings converts a *commonpb.Payloads object into a slice of human-readable strings.
func (s *DataConverter) ToStrings(payloads *commonpb.Payloads) []string {
	var result []string
	for _, payload := range payloads.Payloads {
		result = append(result, s.ToString(payload))
	}
	return result
}

// ToString converts a single Payload to a human-readable string.
func (s *DataConverter) ToString(payload *commonpb.Payload) string {
	// Attempt to deserialize the payload data into a generic interface
	var data interface{}
	if err := json.Unmarshal(payload.GetData(), &data); err != nil {
		// If deserialization fails, return the raw data as a string
		return string(payload.GetData())
	}
	// Convert the deserialized data to a pretty-printed JSON string
	readableStr, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		// If pretty-printing fails, return the raw data as a string
		return string(payload.GetData())
	}
	return string(readableStr)
}

// ToPayloads converts input values to Temporal's Payloads format
func (s *DataConverter) ToPayloads(values ...interface{}) (*commonpb.Payloads, error) {
	payloads := &commonpb.Payloads{}
	for _, v := range values {
		payload, err := s.ToPayload(v)
		if err != nil {
			return nil, err
		}
		payloads.Payloads = append(payloads.Payloads, payload)
	}
	return payloads, nil
}

// FromPayloads converts Temporal Payloads back into Go types
func (s *DataConverter) FromPayloads(payloads *commonpb.Payloads, to ...interface{}) error {
	for i := 0; i < len(to); i++ {
		if i >= len(payloads.Payloads) {
			return io.EOF
		}
		err := s.FromPayload(payloads.Payloads[i], to[i])
		if err != nil {
			return err
		}
	}
	return nil
}

// ToPayload converts a single Go value to a Temporal Payload
func (s *DataConverter) ToPayload(value interface{}) (*commonpb.Payload, error) {
	if value != nil {
		switch v := value.(type) {
		case []byte:
			return &commonpb.Payload{Data: v}, nil
		case starlark.Bytes:
			return &commonpb.Payload{Data: []byte(v)}, nil
		}
	}
	var buf bytes.Buffer
	b, err := star.Encode(value) // Try star encoder first
	if _, ok := err.(star.UnsupportedTypeError); ok {
		b, err = jsoniter.Marshal(value) // Fallback to Go JSON encoder
	}
	if err != nil {
		s.Logger.Error("encode-error", ext.ZapError(err)...)
		return nil, err
	}
	buf.Write(b)
	buf.WriteByte(delimiter)
	return &commonpb.Payload{Data: buf.Bytes()}, nil
}

// FromPayload converts a single Temporal Payload back to a Go value
func (s *DataConverter) FromPayload(payload *commonpb.Payload, to interface{}) error {
	//if to != nil {
	//	switch to := to.(type) {
	//	case *[]byte:
	//		*to = payload.Data
	//		return nil
	//	case *starlark.Bytes:
	//		*to = starlark.Bytes(payload.Data)
	//		return nil
	//	}
	//}
	r := bufio.NewReader(bytes.NewReader(payload.Data))
	line, err := r.ReadBytes(delimiter)
	if err != nil && err != io.EOF {
		s.Logger.Error("decode-error", ext.ZapError(err)...)
		return err
	}
	if len(line) > 0 && line[len(line)-1] == delimiter {
		line = line[:len(line)-1]
	}

	err = star.Decode(line, to)
	if _, ok := err.(star.UnsupportedTypeError); ok {
		err = jsoniter.Unmarshal(line, to)
	}
	if err != nil {
		s.Logger.Error("decode-error", ext.ZapError(err)...)
		return err
	}
	return nil
}
