# protoc-gen-kubeconversion

`protoc-gen-kubeconversion` is a **protoc plugin** that generates Go code to convert between Kubernetes CRD versions.

The goal is to **avoid hand-written converters** and ensure mappings are
**checked at compile time** so schema drift doesn’t silently corrupt data.

---

## Features
- **Go code generation**:
  - Generate go functions to convert between Kubernetes CRD versions.
  - The generated code is compatible with [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) library, and can be used with the controller-runtime's version conversion webhook server.
  - Compatible with Bazel `go_proto_compiler` / `go_proto_library` rules.
  - Generates one `*.convert.pb.go` file per `.proto` file in the current package.
  - The conversion logic can be customized with protobuf options below.

- **Hub & Spoke versions**:
  - The generated code follows the Hub & Spoke model in version conversion ([Conversion concepts](https://book.kubebuilder.io/multiversion-tutorial/conversion-concepts)).
  - For spoke versions (where `version != hub_version`), the generated code implements [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime)'s conversion.Convertible interface.
  - For hub versions (where `version == hub_version`), the generated code implements [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime)'s conversion.Hub interface.

- **Conversion rules**:
  - Fields with the same name and compatible type are auto-mapped.
  - Renamed or moved fields can be mapped via Protobuf options.
  - Unmapped fields must be explicitly ignored; otherwise a build error is raised.
  - Scalar fields are directly assigned.
  - Enum fields are converted by the numeric values (the enum string values do not need to match in different versions).
  - For message fields: if the type is in the same proto package, values are converted via the generated conversion functions; if the type is from a different package, values are deep‑copied.
  - For repeated/map fields: elements/values are converted in the same way as scalar/enum/message fields.
  - Conversion functions are only generated for the CRD messages that are explicitly marked (michelangelo.api.resource.conversion=true) or are referenced (directly or transitively) by the marked messages. Other messages are ignored.

---

## Protobuf Options

### Package-level
- **hub_version**

  Specifies the hub version. There can only be one hub version in the system.


```proto
...
// make sure the hub version is imported in the spoke versions' groupversion_info.proto files, as protoc-gen-convert needs the hub version
// schema to generate conversion functions
import "michelangelo/api/v2/groupversion_info.proto";

option (michelangelo.api.group_info) = {
  name: "michelangelo.api";
  version: "v1"; // version of the current package
  hub_version: "v2"; // hub version
};
```


### Message-level
- **conversion**

  When this option is set, protoc-gen-kubeconversion generates the Go conversion code for this message type and the message types that are referenced by this message type.

```proto
message Project {
  option (michelangelo.api.resource) = {
    conversion: true;
  };
  ...
}
```

- **rename_to**

  By default, message types are converted to/from the message types of the same name in the hub version. This option maps the current message type to a message type in the hub version with a different name.

```proto
message A {
  option (michelangelo.api.rename_to) = "B"; // Message type A maps to message type B in the hub version
  ...
}
```


- **ignore_unmapped_hub_fields**
  By default, protoc-gen-kubeconversion returns a build error if any field in the hub version message is not mapped to a field in the spoke version message (same name, or specified with the `field_rename_to` option). This option specifies a list of field names in the hub version message to ignore.


### Field-level

- **ignore_unmapped**

  Indicates that the field is not mapped in the hub version. When this option is set, the field will be ignored when converting to/from the hub version.

- **field_rename_to**

  Specifies the new name of this field in the hub version.

```proto
string legacy_note = 2 [deprecated=true, (michelangelo.api.ignore_unmapped) = true];
int32 a = 3 [(michelangelo.api.field_rename_to) = "b"];
```

### Enum-level

- **rename_to**
  Specifies the new name of this enum type in the hub version.


```proto
enum State {
  option (michelangelo.api.rename_to) = "ServerState";

  UNKNOWN = 0;
  RUNNING = 1;
  FAILED  = 2;
}
```
