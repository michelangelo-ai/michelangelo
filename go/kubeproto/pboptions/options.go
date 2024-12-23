package pboptions

import (
	"reflect"
	"strconv"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
	golangproto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
)

// Read options from protobuf file
// https://github.com/golang/protobuf/issues/1260

func registerAllExtensions(extTypes *protoregistry.Types, descs interface {
	Messages() protoreflect.MessageDescriptors
	Extensions() protoreflect.ExtensionDescriptors
}) error {
	mds := descs.Messages()
	for i := 0; i < mds.Len(); i++ {
		registerAllExtensions(extTypes, mds.Get(i))
	}
	xds := descs.Extensions()
	for i := 0; i < xds.Len(); i++ {
		if err := extTypes.RegisterExtension(dynamicpb.NewExtensionType(xds.Get(i))); err != nil {
			return err
		}
	}
	return nil
}

// LoadPBExtensions load extension types from protobuf files
func LoadPBExtensions(files []*protogen.File) *protoregistry.Types {
	// The type information for all extensions is in the source files,
	// so we need to extract them into a dynamically created protoregistry.Types.
	extTypes := new(protoregistry.Types)
	for _, file := range files {
		if err := registerAllExtensions(extTypes, file.Desc); err != nil {
			panic(err)
		}
	}

	return extTypes
}

// Options protobuf options of a message
type Options struct {
	OptionsMap map[string]protoreflect.Value
}

// Bool gets a bool option, returns false if not set
func (options *Options) Bool(key string) bool {
	v := options.OptionsMap[key]
	if !v.IsValid() {
		return false
	}
	return v.Bool()
}

// String gets a string option, returns "" if not set
func (options *Options) String(key string) string {
	v := options.OptionsMap[key]
	if !v.IsValid() {
		return ""
	}
	return v.String()
}

// Int64 gets an int64 option, returns 0 if not set
func (options *Options) Int64(key string) int64 {
	v := options.OptionsMap[key]
	if !v.IsValid() {
		return 0
	}
	return v.Int()
}

// GetSubOptions returns a subset of options
func (options *Options) GetSubOptions(key string) *Options {
	newOptions := &Options{make(map[string]protoreflect.Value)}
	for k := range options.OptionsMap {
		if strings.HasPrefix(k, key+".") {
			newOptions.OptionsMap[strings.Replace(k, key+".", "", 1)] = options.OptionsMap[k]
		}
	}
	return newOptions
}

// ReadOptions read protobuf options
func ReadOptions(extTypes *protoregistry.Types, pbOptions interface {
	ProtoReflect() protoreflect.Message
	Reset()
}) (*Options, error) {
	options := Options{make(map[string]protoreflect.Value)}
	if pbOptions != nil && !reflect.ValueOf(pbOptions).IsNil() {
		b, err := golangproto.Marshal(pbOptions)
		if err != nil {
			return nil, err
		}
		pbOptions.Reset()
		err = golangproto.UnmarshalOptions{Resolver: extTypes}.Unmarshal(b, pbOptions)
		if err != nil {
			return nil, err
		}

		// Use protobuf reflection to iterate over all the extension fields,
		// looking for the ones that we are interested in.
		pbOptions.ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
			if !fd.IsExtension() {
				return true
			}
			if strings.HasPrefix(string(fd.FullName()), "michelangelo.api") {
				options.readField("", string(fd.Name()), v)
			}

			return true
		})
	}
	return &options, nil
}

func (options *Options) readField(prefix string, name string, v protoreflect.Value) {
	msg, isMsg := v.Interface().(protoreflect.Message)
	list, isList := v.Interface().(protoreflect.List)
	if isMsg {
		options.OptionsMap[prefix+"has_"+name] = protoreflect.ValueOfBool(true)
		msg.Range(func(fd1 protoreflect.FieldDescriptor, v1 protoreflect.Value) bool {
			options.readField(prefix+name+".", string(fd1.Name()), v1)
			return true
		})
	} else if isList {
		options.OptionsMap[prefix+"len("+name+")"] = protoreflect.ValueOfInt64(int64(list.Len()))
		for i := 0; i < list.Len(); i++ {
			fieldName := name + "[" + strconv.Itoa(i) + "]"
			options.readField(prefix, fieldName, list.Get(i))
		}
	} else {
		options.OptionsMap[prefix+name] = v
	}
}
