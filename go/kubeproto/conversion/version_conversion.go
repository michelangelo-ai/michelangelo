package conversion

import (
	"fmt"
	"log"
	"os"
	"sort"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"

	"github.com/michelangelo-ai/michelangelo/go/kubeproto/groupinfo"
	"github.com/michelangelo-ai/michelangelo/go/kubeproto/pboptions"
	"github.com/michelangelo-ai/michelangelo/go/kubeproto/util"
)

var logger = log.New(os.Stderr, "", 0)

// collectTargets collects all messages that needs conversion in the given package, including the CRD messages that are
// marked with `resource.conversion` option and their dependencies.
func collectTargets(gen *protogen.Plugin, pkg string, extTypes *protoregistry.Types) map[string]*protogen.Message {
	messages := map[string]*protogen.Message{}
	for _, file := range gen.Files {
		if string(file.Desc.Package()) != pkg {
			continue
		}

		queue := []*protogen.Message{}
		for _, m := range file.Messages {
			opts, err := pboptions.ReadOptions(extTypes, m.Desc.Options().(*descriptorpb.MessageOptions))
			if err != nil {
				continue
			}
			if opts.Bool("resource.conversion") {
				messages[m.GoIdent.GoName] = m
				queue = append(queue, m)
			}
		}
		if len(messages) == 0 {
			continue
		}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for _, f := range cur.Fields {
				if f.Desc.Kind() == protoreflect.MessageKind {
					t := f.Message
					if f.Desc.IsMap() {
						// in protobuf, map key cannot be a message type
						// we only check if the value type is a message type
						if f.Message.Fields[1].Desc.Kind() == protoreflect.MessageKind {
							t = f.Message.Fields[1].Message
						} else {
							continue
						}
					}
					if t == nil || !sameLocalPackage(file, t) {
						continue
					}
					if _, ok := messages[t.GoIdent.GoName]; !ok {
						messages[t.GoIdent.GoName] = t
						queue = append(queue, t)
					}
				}
			}
		}
	}
	return messages
}

func getMessageRename(extTypes *protoregistry.Types, m *protogen.Message) string {
	opts, err := pboptions.ReadOptions(extTypes, m.Desc.Options().(*descriptorpb.MessageOptions))
	if err != nil {
		return ""
	}
	r := opts.String("rename_to")
	return r
}

func getEnumRename(extTypes *protoregistry.Types, e *protogen.Enum) string {
	opts, err := pboptions.ReadOptions(extTypes, e.Desc.Options().(*descriptorpb.EnumOptions))
	if err != nil {
		return ""
	}
	return opts.String("rename_to")
}

func getFieldRename(extTypes *protoregistry.Types, f *protogen.Field) string {
	opts, err := pboptions.ReadOptions(extTypes, f.Desc.Options().(*descriptorpb.FieldOptions))
	if err != nil {
		return ""
	}
	return opts.String("field_rename_to")
}

func fieldIgnored(extTypes *protoregistry.Types, f *protogen.Field) bool {
	opts, err := pboptions.ReadOptions(extTypes, f.Desc.Options().(*descriptorpb.FieldOptions))
	if err != nil {
		return false
	}
	return opts.Bool("ignore_unmapped")
}

func findHubMessage(gen *protogen.Plugin, hubProtoPkg, hubMsgName string) *protogen.Message {
	for _, f := range gen.Files {
		if string(f.Desc.Package()) != hubProtoPkg {
			continue
		}
		for _, m := range f.Messages {
			if string(m.Desc.Name()) == hubMsgName {
				return m
			}
		}
	}
	return nil
}

func goScalarType(kind protoreflect.Kind) string {
	switch kind {
	case protoreflect.BoolKind:
		return "bool"
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return "int32"
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return "int64"
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return "uint32"
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return "uint64"
	case protoreflect.FloatKind:
		return "float32"
	case protoreflect.DoubleKind:
		return "float64"
	case protoreflect.StringKind:
		return "string"
	case protoreflect.BytesKind:
		return "[]byte"
	default:
		return ""
	}
}

func typeMismatch(file *protogen.File, spokeMsg *protogen.Message, hubMsg *protogen.Message, sf *protogen.Field, hf *protogen.Field) {
	logger.Panic(fmt.Sprintf("%s: incompatible field types for conversion %s.%s (%s) <-> %s.%s (%s)",
		file.Desc.Path(),
		spokeMsg.GoIdent.GoName, sf.GoName, getGoTypeName(sf),
		hubMsg.GoIdent.GoName, hf.GoName, getGoTypeName(hf)))
}

func getGoTypeName(field *protogen.Field) string {
	goTypeName := func(field *protogen.Field) string {
		if field.Desc.Kind() == protoreflect.MessageKind {
			return field.Message.GoIdent.GoName
		} else if field.Desc.Kind() == protoreflect.EnumKind {
			return field.Enum.GoIdent.GoName
		} else {
			return goScalarType(field.Desc.Kind())
		}
	}
	if field.Desc.IsMap() {
		return fmt.Sprintf("map[%v]%v", goTypeName(field.Message.Fields[0]), goTypeName(field.Message.Fields[1]))
	} else if field.Desc.IsList() {
		return fmt.Sprintf("[]%v", goTypeName(field))
	}
	return goTypeName(field)
}

func isSameExternalMessage(sf, hf *protogen.Field) bool {
	return sf.Message != nil && hf.Message != nil && sf.Message.GoIdent == hf.Message.GoIdent
}

func readIgnoreUnmappedHubFields(extTypes *protoregistry.Types, m *protogen.Message) map[string]struct{} {
	opts, err := pboptions.ReadOptions(extTypes, m.Desc.Options().(*descriptorpb.MessageOptions))
	if err != nil {
		return nil
	}
	n := int(opts.Int64("len(ignore_unmapped_hub_fields)"))
	if n <= 0 {
		return nil
	}
	res := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		name := opts.String(fmt.Sprintf("ignore_unmapped_hub_fields[%d]", i))
		if name != "" {
			res[name] = struct{}{}
		}
	}
	return res
}

func generateAssignmentsSpokeToHub(g *protogen.GeneratedFile, file *protogen.File, extTypes *protoregistry.Types, spokeMsg *protogen.Message, hubMsg *protogen.Message) {
	hubFields := map[string]*protogen.Field{}
	for _, hf := range hubMsg.Fields {
		hubFields[string(hf.Desc.Name())] = hf
	}
	mapped := map[string]struct{}{} // hub fields that are mapped to a spoke field
	for _, sf := range spokeMsg.Fields {
		if fieldIgnored(extTypes, sf) {
			continue
		}
		targetName := string(sf.Desc.Name())
		if rn := getFieldRename(extTypes, sf); rn != "" {
			targetName = rn
		}
		hf := hubFields[targetName]
		if hf == nil {
			continue // no matching hub field
		}
		generateFieldAssignment(g, file, extTypes, spokeMsg, hubMsg, sf, hf, true)
		mapped[targetName] = struct{}{}
	}
	// Enforce unmapped hub fields unless explicitly ignored
	ignore := readIgnoreUnmappedHubFields(extTypes, spokeMsg) // hub fields that are explicitly ignored
	for _, hf := range hubMsg.Fields {
		name := string(hf.Desc.Name())
		if _, ok := mapped[name]; ok {
			continue
		}
		if _, ok := ignore[name]; ok {
			continue
		}
		logger.Panic(fmt.Sprintf("%s: hub field '%s' is not mapped from spoke '%s' and not listed in ignore_unmapped_hub_fields", file.Desc.Path(), name, spokeMsg.GoIdent.GoName))
	}
}

func generateAssignmentsHubToSpoke(g *protogen.GeneratedFile, file *protogen.File, extTypes *protoregistry.Types, spokeMsg *protogen.Message, hubMsg *protogen.Message) {
	rev := map[string]*protogen.Field{}
	for _, sf := range spokeMsg.Fields {
		if fieldIgnored(extTypes, sf) {
			continue
		}
		hName := string(sf.Desc.Name())
		if rn := getFieldRename(extTypes, sf); rn != "" {
			hName = rn
		}
		rev[hName] = sf
	}
	for _, hf := range hubMsg.Fields {
		sf := rev[string(hf.Desc.Name())]
		if sf == nil {
			continue
		}
		generateFieldAssignment(g, file, extTypes, spokeMsg, hubMsg, sf, hf, false)
	}
}

func sameLocalPackage(file *protogen.File, m *protogen.Message) bool {
	if m == nil {
		return false
	}
	return string(m.Desc.ParentFile().Package()) == string(file.Desc.Package())
}

func generateFieldAssignment(g *protogen.GeneratedFile, file *protogen.File, extTypes *protoregistry.Types, spokeMsg *protogen.Message, hubMsg *protogen.Message,
	sf *protogen.Field, hf *protogen.Field, spokeToHub bool) {
	var in, out string
	var outField *protogen.Field
	if spokeToHub {
		in = "in." + sf.GoName
		out = "out." + hf.GoName
		outField = hf
	} else {
		in = "in." + hf.GoName
		out = "out." + sf.GoName
		outField = sf
	}

	convertName := func(msgName string) string {
		if spokeToHub {
			return "Convert" + msgName + "ToHub"
		}
		return "Convert" + msgName + "FromHub"
	}

	// Single field & List
	if !sf.Desc.IsMap() {
		if (sf.Desc.IsList() && !hf.Desc.IsList()) || (!sf.Desc.IsList() && hf.Desc.IsList()) {
			typeMismatch(file, spokeMsg, hubMsg, sf, hf)
		}

		switch sf.Desc.Kind() {
		case protoreflect.MessageKind:
			if hf.Desc.Kind() != protoreflect.MessageKind {
				typeMismatch(file, spokeMsg, hubMsg, sf, hf)
			}
			if sameLocalPackage(file, sf.Message) {
				msgName := sf.Message.GoIdent.GoName
				if r := getMessageRename(extTypes, spokeMsg); r != "" {
					msgName = r
				}
				if msgName != hf.Message.GoIdent.GoName {
					typeMismatch(file, spokeMsg, hubMsg, sf, hf)
				}
				g.P("if ", in, " == nil {")
				g.P("\t", out, " = nil")
				g.P("} else {")
				if sf.Desc.IsList() {
					g.P("if ", in, " == nil {")
					g.P("\t", out, " = nil")
					g.P("} else {")
					g.P("\t", out, " = []*", g.QualifiedGoIdent(outField.Message.GoIdent), "{}")
					g.P("}")
					g.P("for _, v := range ", in, " {")
					g.P("\tt := &", g.QualifiedGoIdent(outField.Message.GoIdent), "{}")
					g.P("\t", convertName(sf.Message.GoIdent.GoName), "(v, t)")
					g.P("\t", out, " = append(", out, ", t)")
					g.P("}")
				} else {
					g.P(out, " = &", g.QualifiedGoIdent(outField.Message.GoIdent), "{}")
					g.P(convertName(sf.Message.GoIdent.GoName), "(", in, ", ", out, ")")
				}
				g.P("}")
				return
			}
			if isSameExternalMessage(sf, hf) {
				// Deep-clone identical external message types
				protoClone := protogen.GoIdent{GoImportPath: "google.golang.org/protobuf/proto", GoName: "Clone"}
				if sf.Desc.IsList() {
					g.P("if ", in, " == nil {")
					g.P("\t", out, " = nil")
					g.P("} else {")
					g.P("\t", out, " = []*", g.QualifiedGoIdent(outField.Message.GoIdent), "{}")
					g.P("}")
					g.P("for _, v := range ", in, " {")
					g.P("\t", out, " = append(", out, ", ", g.QualifiedGoIdent(protoClone), "(v).(*", g.QualifiedGoIdent(outField.Message.GoIdent), "))")
					g.P("}")
				} else {
					g.P(out, " = ", g.QualifiedGoIdent(protoClone), "(", in, ").(*", g.QualifiedGoIdent(outField.Message.GoIdent), ")")
				}
				return
			}
			typeMismatch(file, spokeMsg, hubMsg, sf, hf)
		case protoreflect.EnumKind:
			if hf.Desc.Kind() != protoreflect.EnumKind {
				typeMismatch(file, spokeMsg, hubMsg, sf, hf)
			}
			spokeEnum := sf.Enum
			hubEnum := hf.Enum
			// check if enum types match
			if rn := getEnumRename(extTypes, spokeEnum); rn != "" { // if enum is renamed
				if rn != string(hubEnum.Desc.Name()) {
					typeMismatch(file, spokeMsg, hubMsg, sf, hf)
				}
			} else if spokeEnum.Desc.Name() != hubEnum.Desc.Name() {
				typeMismatch(file, spokeMsg, hubMsg, sf, hf)
			}
			if sf.Desc.IsList() {
				g.P("if ", in, " == nil {")
				g.P("\t", out, " = nil")
				g.P("} else {")
				g.P("\t", out, " = []", g.QualifiedGoIdent(outField.Enum.GoIdent), "{}")
				g.P("}")
				g.P("for _, v := range ", in, " {")
				g.P("\t", out, " = append(", out, ", ", g.QualifiedGoIdent(outField.Enum.GoIdent), "(int32(v)))")
				g.P("}")
			} else {
				g.P(out, " = ", g.QualifiedGoIdent(outField.Enum.GoIdent), "(int32(", in, "))")
			}
		case protoreflect.BoolKind, protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
			protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind,
			protoreflect.Uint32Kind, protoreflect.Fixed32Kind,
			protoreflect.Uint64Kind, protoreflect.Fixed64Kind,
			protoreflect.FloatKind, protoreflect.DoubleKind,
			protoreflect.StringKind, protoreflect.BytesKind:
			if sf.Desc.Kind() != hf.Desc.Kind() {
				typeMismatch(file, spokeMsg, hubMsg, sf, hf)
			}
			if sf.Desc.IsList() {
				g.P("if ", in, " == nil {")
				g.P("\t", out, " = nil")
				g.P("} else {")
				g.P("\t", out, " = []", goScalarType(outField.Desc.Kind()), "{}")
				g.P("}")
				g.P("for _, v := range ", in, " {")
				g.P("\t", out, " = append(", out, ", v)")
				g.P("}")
			} else {
				g.P(out, " = ", in)
			}
		default:
			typeMismatch(file, spokeMsg, hubMsg, sf, hf)
		}
		return
	}

	// Map field
	if sf.Desc.IsMap() {
		if !hf.Desc.IsMap() {
			typeMismatch(file, spokeMsg, hubMsg, sf, hf)
		}
		if sf.Desc.MapKey().Kind() != hf.Desc.MapKey().Kind() {
			typeMismatch(file, spokeMsg, hubMsg, sf, hf)
		}

		kKind := outField.Desc.MapKey().Kind()
		vKind := outField.Desc.MapValue().Kind()
		keyType := goScalarType(kKind) // in protobuf, map key can only be integer or string
		valType := ""
		if vKind == protoreflect.MessageKind {
			valType = "*" + g.QualifiedGoIdent(outField.Message.Fields[1].Message.GoIdent)
		} else if vKind == protoreflect.EnumKind {
			valType = g.QualifiedGoIdent(outField.Message.Fields[1].Enum.GoIdent)
		} else {
			if sf.Desc.MapValue().Kind() != hf.Desc.MapValue().Kind() {
				typeMismatch(file, spokeMsg, hubMsg, sf, hf)
			}
			valType = goScalarType(vKind)
		}
		g.P("if ", in, " == nil {")
		g.P("\t", out, " = nil")
		g.P("} else {")
		g.P("\t", out, " = make(map[", keyType, "]", valType, ", len(", in, "))")
		g.P("\tfor k, v := range ", in, " {")
		if vKind == protoreflect.MessageKind {
			if hf.Desc.MapValue().Kind() != protoreflect.MessageKind {
				typeMismatch(file, spokeMsg, hubMsg, sf, hf)
			}
			if sameLocalPackage(file, sf.Message.Fields[1].Message) {
				vMsgName := sf.Message.Fields[1].Message.GoIdent.GoName
				if r := getMessageRename(extTypes, spokeMsg); r != "" {
					vMsgName = r
				}
				if vMsgName != hf.Message.Fields[1].Message.GoIdent.GoName {
					typeMismatch(file, spokeMsg, hubMsg, sf, hf)
				}
				g.P("\t\tif v == nil {")
				g.P("\t\t\t", out, "[k] = nil")
				g.P("\t\t\tcontinue")
				g.P("\t\t}")
				g.P("\t\tt := &", g.QualifiedGoIdent(outField.Message.Fields[1].Message.GoIdent), "{}")
				g.P("\t\t", convertName(sf.Message.Fields[1].Message.GoIdent.GoName), "(v,t)")
				g.P("\t\t", out, "[k] = t")
			} else if isSameExternalMessage(sf.Message.Fields[1], hf.Message.Fields[1]) {
				// Clone value for identical external message types
				protoClone := protogen.GoIdent{GoImportPath: "google.golang.org/protobuf/proto", GoName: "Clone"}
				g.P("\t\t", out, "[k] = ", g.QualifiedGoIdent(protoClone), "(v).(*",
					g.QualifiedGoIdent(sf.Message.Fields[1].Message.GoIdent), ")")
			} else {
				typeMismatch(file, spokeMsg, hubMsg, sf, hf)
			}
		} else if vKind == protoreflect.EnumKind {
			if hf.Desc.MapValue().Kind() != protoreflect.EnumKind {
				typeMismatch(file, spokeMsg, hubMsg, sf, hf)
			}
			g.P("\t\t", out, "[k] = ", valType, "(int32(v))")
		} else {
			g.P("\t\t", out, "[k] = v")
		}
		g.P("\t}")
		g.P("}")
		return
	}
}

func generateFileSpoke(gen *protogen.Plugin, targets map[string]*protogen.Message, file *protogen.File,
	extTypes *protoregistry.Types, hubProtoPkg string) {
	filename := file.GeneratedFilenamePrefix + ".conversion.pb.go"
	g := gen.NewGeneratedFile(filename, file.GoImportPath)
	g.P("// Code generated by protoc-gen-kubeconversion. DO NOT EDIT.")
	g.P("package ", file.GoPackageName)
	g.P()

	names := make([]string, 0, len(targets))
	for name := range targets {
		spokeMsg := targets[name]
		if spokeMsg.Desc.ParentFile().Path() != file.Desc.Path() { // skip the messages that are not in the current file
			continue
		}
		names = append(names, name)
	}
	// Deterministic order
	sort.Strings(names)

	for _, name := range names {
		spokeMsg := targets[name]
		hubName := name
		if r := getMessageRename(extTypes, spokeMsg); r != "" {
			hubName = r
		}
		hubMsg := findHubMessage(gen, hubProtoPkg, hubName)
		// If hub message not found, surface a hard error to the plugin caller.
		if hubMsg == nil {
			logger.Panic(fmt.Sprintf("%s: hub message '%s.%s' not found for spoke message '%s.%s'. "+
				"Ensure hub package '%s' is imported.", file.Desc.Path(), hubProtoPkg, hubName, *file.Proto.Package, name, hubProtoPkg))
		}

		opts, err := pboptions.ReadOptions(extTypes, spokeMsg.Desc.Options().(*descriptorpb.MessageOptions))
		if err != nil {
			continue
		}
		if opts.Bool("has_resource") { // is a CRD type
			hubTypeIndet := protogen.GoIdent{GoImportPath: "sigs.k8s.io/controller-runtime/pkg/conversion", GoName: "Hub"}
			g.P("func (src *", g.QualifiedGoIdent(spokeMsg.GoIdent), ") ConvertTo(dstRaw ", hubTypeIndet, ") error {")
			g.P("dst := dstRaw.(*", g.QualifiedGoIdent(hubMsg.GoIdent), ")")
			g.P("if err := Convert", spokeMsg.GoIdent.GoName, "SpecToHub(&src.Spec, &dst.Spec); err != nil {")
			g.P("return err")
			g.P("}")
			g.P("if err := Convert", spokeMsg.GoIdent.GoName, "StatusToHub(&src.Status, &dst.Status); err != nil {")
			g.P("return err")
			g.P("}")
			g.P("return nil")
			g.P("}")
			g.P()

			g.P("func (dst *", g.QualifiedGoIdent(spokeMsg.GoIdent), ") ConvertFrom(srcRaw ", hubTypeIndet, ") error {")
			g.P("src := srcRaw.(*", g.QualifiedGoIdent(hubMsg.GoIdent), ")")
			g.P("if err := Convert", spokeMsg.GoIdent.GoName, "SpecFromHub(&src.Spec, &dst.Spec); err != nil {")
			g.P("return err")
			g.P("}")
			g.P("if err := Convert", spokeMsg.GoIdent.GoName, "StatusFromHub(&src.Status, &dst.Status); err != nil {")
			g.P("return err")
			g.P("}")
			g.P("return nil")
			g.P("}")
			g.P()
		} else {
			// Forward
			g.P("func Convert", name, "ToHub", "(in *", name, ", out *", g.QualifiedGoIdent(hubMsg.GoIdent), ") error {")
			g.P("if in == nil { return nil }")
			generateAssignmentsSpokeToHub(g, file, extTypes, spokeMsg, hubMsg)
			g.P("return nil")
			g.P("}")
			g.P()
			// Reverse
			g.P("func Convert", name, "FromHub", "(in *", g.QualifiedGoIdent(hubMsg.GoIdent), ", out *", name, ") error {")
			g.P("if in == nil { return nil }")
			generateAssignmentsHubToSpoke(g, file, extTypes, spokeMsg, hubMsg)
			g.P("return nil")
			g.P("}")
			g.P()
		}
	}
}

func generateFileHub(gen *protogen.Plugin, file *protogen.File, extTypes *protoregistry.Types) {
	var crdMessages []*protogen.Message
	for _, m := range file.Messages {
		opts, err := pboptions.ReadOptions(extTypes, m.Desc.Options().(*descriptorpb.MessageOptions))
		if err != nil {
			continue
		}
		if opts.Bool("resource.conversion") {
			crdMessages = append(crdMessages, m)
		}
	}
	if len(crdMessages) == 0 {
		return
	}

	filename := file.GeneratedFilenamePrefix + ".conversion.pb.go"
	g := gen.NewGeneratedFile(filename, file.GoImportPath)
	g.P("// Code generated by protoc-gen-kubeconversion. DO NOT EDIT.")
	g.P("package ", file.GoPackageName)
	g.P()
	for _, m := range crdMessages {
		g.P("func (*", m.GoIdent, ") Hub() {}")
		g.P()
	}
}

func Generate(reqData []byte) *pluginpb.CodeGeneratorResponse {
	gen, extTypes, err := util.GetPluginAndExtensions(reqData, false)
	if err != nil {
		logger.Panic(err)
	}

	// Load group info
	gInfoMap := groupinfo.Load(gen, extTypes)

	// group files to generate by package
	// normally, this protoc plugin is called once for each package to generate the go code
	filesToGenerate := make(map[string][]*protogen.File)
	for _, file := range gen.Files {
		if !file.Generate {
			continue
		}
		filesToGenerate[string(file.Desc.Package())] = append(filesToGenerate[string(file.Desc.Package())], file)
	}

	for pkg, files := range filesToGenerate {
		gInfo, ok := gInfoMap[pkg]
		if !ok {
			logger.Panic(fmt.Sprintf("hub version info not found for package %s. "+
				"Make sure to define groupversion_info.proto for the API group.", pkg))
		}
		if gInfo.HubVersion != gInfo.Version { // spoke version
			// find all messages that needs conversion in the package
			targets := collectTargets(gen, pkg, extTypes)
			if len(targets) == 0 {
				continue
			}

			var hubProtoPkg string
			for protoPkg, v := range gInfoMap {
				if v.Name != gInfo.Name || v.Version != gInfo.HubVersion {
					continue
				}
				if v.HubVersion != gInfo.HubVersion {
					logger.Panic(fmt.Sprintf("Inconsistent hub version settings in %s and %s. ", gInfo.File.Desc.Path(), v.File.Desc.Path()))
				}
				hubProtoPkg = protoPkg
			}
			if hubProtoPkg == "" {
				logger.Panic(fmt.Sprintf("hub protobuf package (%s, %s) not found for package %s. "+
					"Make sure to import the hub version's groupversion_info.proto file "+
					"in spoke version's groupversion_info.proto file.", gInfo.Name, gInfo.HubVersion, pkg))
			}
			for _, file := range files {
				generateFileSpoke(gen, targets, file, extTypes, hubProtoPkg)
			}
		} else { // hub version
			for _, file := range files {
				generateFileHub(gen, file, extTypes)
			}
		}
	}

	return gen.Response()
}
