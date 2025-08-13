package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/michelangelo-ai/michelangelo/go/kubeproto/pboptions"
	"github.com/michelangelo-ai/michelangelo/go/kubeproto/templates"
	"github.com/michelangelo-ai/michelangelo/go/kubeproto/util"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

var logger = log.New(os.Stderr, "", 0)

// Options for the ext compiler
type ExtOptions struct {
	OriginalProto string // Reference to the original proto file for comparison
}

func parseOptions(req *pluginpb.CodeGeneratorRequest) *ExtOptions {
	opts := &ExtOptions{}
	if req.Parameter != nil && *req.Parameter != "" {
		params := strings.Split(*req.Parameter, ",")
		for _, param := range params {
			kv := strings.SplitN(param, "=", 2)
			if len(kv) == 2 {
				switch kv[0] {
				case "original_proto":
					opts.OriginalProto = kv[1]
				}
			}
		}
	}
	return opts
}

func generateFile(gen *protogen.Plugin, file *protogen.File, extTypes *protoregistry.Types, opts *ExtOptions, allFiles []*protogen.File) *protogen.GeneratedFile {
	// Check if this file has ext_original_proto option
	fileOptions := file.Desc.Options().(*descriptorpb.FileOptions)
	pbFileOptions, err := pboptions.ReadOptions(extTypes, fileOptions)
	if err == nil && pbFileOptions.String("ext_original_proto") != "" {
		originalProtoPath := pbFileOptions.String("ext_original_proto")
		// Find the original proto file
		var originalFile *protogen.File
		for _, f := range allFiles {
			if strings.Contains(f.Desc.Path(), strings.TrimPrefix(originalProtoPath, "michelangelo/")) {
				originalFile = f
				break
			}
		}

		if originalFile != nil {
			// Verify fields match before generation
			if err := verifyProtoMatch(file, originalFile); err != nil {
				logger.Printf("Warning: Proto verification failed for %s: %v", file.Desc.Path(), err)
				// Continue with generation but log the warning
			}
		}
	}

	// Generate .ext.go file in the same package but with ext suffix for clarity
	// This avoids package conflicts while keeping things simple
	filename := file.GeneratedFilenamePrefix + ".ext.go"
	g := gen.NewGeneratedFile(filename, file.GoImportPath)

	// Write file header with package name
	header := fmt.Sprintf(templates.FileHeader, "protoc-gen-ext", file.GoPackageName)
	g.P(header)

	// No need to import since we're in the same package
	g.P()

	// Global counter for pattern and set variables to ensure uniqueness
	globalPatternCounter := 0
	globalSetCounter := 0

	// Collect all patterns and sets first
	allPatterns := []string{}
	allSets := []string{}
	allValidationFuncs := []string{}

	// First pass: collect all patterns, sets, and validation code
	for _, msg := range file.Messages {
		validateCode, patterns, sets, err := generateValidationCodeWithCounters(msg, extTypes, "", &globalPatternCounter, &globalSetCounter)
		if err != nil {
			compilerErrMsg(msg, err.Error())
		}

		if validateCode == "" {
			continue // Skip messages without validation
		}

		// Collect patterns and sets
		allPatterns = append(allPatterns, patterns...)
		allSets = append(allSets, sets...)

		// Generate Validate function for the message type
		var buf bytes.Buffer
		templates.ValidateFuncTmp.Execute(&buf, struct {
			TypeName      string
			ValidateLogic string
		}{msg.GoIdent.GoName, validateCode})
		allValidationFuncs = append(allValidationFuncs, buf.String())
	}

	// Generate all pattern variables at the top
	if len(allPatterns) > 0 {
		for _, pattern := range allPatterns {
			g.P(pattern)
		}
		g.P()
	}

	// Generate all set variables at the top
	if len(allSets) > 0 {
		for _, set := range allSets {
			g.P(set)
		}
		g.P()
	}

		// Generate all validation functions
	for _, validationFunc := range allValidationFuncs {
		g.P(validationFunc)
	}

	// Generate ValidationRegistry and init function if we have validations
	if len(allValidationFuncs) > 0 {
		g.P()
		g.P("// ValidationRegistry holds validation functions for ext types")
		g.P("var ValidationRegistry = make(map[string]func(interface{}, string) error)")
		g.P()
		g.P("// init registers the validation functions")
		g.P("func init() {")
		for _, msg := range file.Messages {
			// Check if message has validation
			tempPatternCounter := 0
			tempSetCounter := 0
			validateCode, _, _, _ := generateValidationCodeWithCounters(msg, extTypes, "", &tempPatternCounter, &tempSetCounter)
			if validateCode != "" {
				// Register the validation function
				g.P(fmt.Sprintf(`	// Register validation for %s`, msg.GoIdent.GoName))
				g.P(fmt.Sprintf(`	ValidationRegistry["%s"] = func(obj interface{}, prefix string) error {`, msg.GoIdent.GoName))
				g.P(fmt.Sprintf(`		if msg, ok := obj.(*%s); ok {`, msg.GoIdent.GoName))
				g.P(`			return msg.Validate(prefix)`)
				g.P(`		}`)
				g.P(`		return nil`)
				g.P(`	}`)
			}
		}
		g.P("}")
		g.P()
		g.P("// Validate validates an object using the registry")
		g.P("func Validate(typeName string, obj interface{}, prefix string) error {")
		g.P(`	if fn, ok := ValidationRegistry[typeName]; ok {`)
		g.P(`		return fn(obj, prefix)`)
		g.P(`	}`)
		g.P(`	return nil`)
		g.P("}")
	}

	return g
}

func generateValidationCodeWithCounters(msg *protogen.Message, extTypes *protoregistry.Types, prefix string, patternCounter *int, setCounter *int) (string, []string, []string, error) {
	return generateValidationCode(msg, extTypes, prefix, patternCounter, setCounter)
}

func generateValidationCode(msg *protogen.Message, extTypes *protoregistry.Types, prefix string, patternCounter *int, setCounter *int) (string, []string, []string, error) {
	var patterns []string
	var sets []string
	validateCode := ""

	// Process message options
	pbOptions := msg.Desc.Options().(*descriptorpb.MessageOptions)
	msgOptions, err := pboptions.ReadOptions(extTypes, pbOptions)
	if err != nil {
		return "", nil, nil, err
	}

	noDefault := false
	if msgOptions.Bool("no_default") {
		noDefault = true
	}

	// Process fields
	for _, field := range msg.Fields {
		pbOptions := field.Desc.Options().(*descriptorpb.FieldOptions)
		options, err := pboptions.ReadOptions(extTypes, pbOptions)
		if err != nil {
			return "", nil, nil, err
		}

		hasValidation := options.Bool("has_validation[0]")

		fieldValidateCode := ""
		itemsValidateCode := ""
		keysValidateCode := ""
		valuesValidateCode := ""

		if !hasValidation {
			if noDefault {
				fieldValidateCode += validateNoDefault(field, nil)
			}
		} else {
			// Process validation rules
			for i := 0; ; i++ {
				validationName := fmt.Sprintf("validation[%d]", i)
				if !options.Bool("has_" + validationName) {
					break
				}
				validation := options.GetSubOptions(validationName)

				// Process various validation types
				if validation.Bool("required") {
					fieldValidateCode += validateNoDefault(field, validation)
				}

				if validation.String("max_items") != "" || validation.String("min_items") != "" {
					fieldValidateCode += validateMaxMinItems(field, validation)
				}

				if validation.String("max_length") != "" || validation.String("min_length") != "" {
					fieldValidateCode += validateMaxMinLength(field, validation)
				}

								// Add pattern tracking
				addPattern := func(pattern string) int {
					idx := *patternCounter
					*patternCounter++
					// Ensure pattern matches the entire string by anchoring with ^ and $
					anchoredPattern := pattern
					if !strings.HasPrefix(pattern, "^") {
						anchoredPattern = "^" + anchoredPattern
					}
					if !strings.HasSuffix(pattern, "$") {
						anchoredPattern = anchoredPattern + "$"
					}
					patterns = append(patterns, fmt.Sprintf("var pattern%d = regexp.MustCompile(%s)", idx, strconv.Quote(anchoredPattern)))
					return idx
				}

				// Add set tracking
				addSet := func(values []string, typ string) int {
					idx := *setCounter
					*setCounter++
					setStr := fmt.Sprintf("var set%d = map[%s]bool{", idx, typ)
					for _, v := range values {
						// Quote string values properly
						if typ == "string" {
							setStr += fmt.Sprintf("%s: true, ", strconv.Quote(v))
						} else {
							setStr += fmt.Sprintf("%s: true, ", v)
						}
					}
					setStr += "}"
					sets = append(sets, setStr)
					return idx
				}

				fieldValidateCode += validateSimpleValue(field, validation, addPattern, addSet, fieldTarget)

				// Handle items, keys, values validation
				if validation.Bool("has_items") {
					itemsValidation := validation.GetSubOptions("items")
					itemsValidateCode += validateSimpleValue(field, itemsValidation, addPattern, addSet, itemTarget)
				}

				if validation.Bool("has_keys") {
					keysValidation := validation.GetSubOptions("keys")
					keysValidateCode += validateSimpleValue(field, keysValidation, addPattern, addSet, keyTarget)
				}

				if validation.Bool("has_values") {
					valuesValidation := validation.GetSubOptions("values")
					valuesValidateCode += validateSimpleValue(field, valuesValidation, addPattern, addSet, valueTarget)
				}
			}
		}

		// Add field validation code
		if fieldValidateCode != "" || itemsValidateCode != "" || keysValidateCode != "" || valuesValidateCode != "" {
			validateCode += generateFieldValidation(field, fieldValidateCode, itemsValidateCode, keysValidateCode, valuesValidateCode)
		}
	}

	// Process oneofs
	for _, oneof := range msg.Oneofs {
		pbOptions := oneof.Desc.Options().(*descriptorpb.OneofOptions)
		options, err := pboptions.ReadOptions(extTypes, pbOptions)
		if err != nil {
			return "", nil, nil, err
		}

		if options.Bool("required") {
			fields := ""
			for j, field := range oneof.Fields {
				if j > 0 {
					fields += ", "
				}
				fields += string(field.Desc.Name())
			}
			validateCode += fmt.Sprintf(templates.ValidateOneofFmt, oneof.GoName, oneof.Desc.Name(), fields)
		}
	}

	return validateCode, patterns, sets, nil
}

// Helper functions for validation
func compilerErrField(field *protogen.Field, errMsg string) {
	logger.Panicf("%s: Error while parsing validation rules for field %s.%s: %s",
		field.Location.SourceFile, field.Parent.Desc.Name(), field.Desc.TextName(), errMsg)
}

func compilerErrMsg(msg *protogen.Message, errMsg string) {
	logger.Panicf("%s: Error while parsing validation rules for message %s: %s",
		msg.Location.SourceFile, msg.Desc.Name(), errMsg)
}

func validateField(validation *pboptions.Options, errMsg string, condition string) string {
	if validation != nil && validation.String("msg") != "" {
		errMsg = validation.String("msg")
	}
	var buf bytes.Buffer
	templates.ValidateFieldTmp.Execute(&buf, struct {
		Condition string
		Msg       string
	}{condition, strconv.Quote(errMsg)})
	return buf.String()
}

// Target types for validation
type targetType int

const (
	fieldTarget targetType = iota
	itemTarget
	keyTarget
	valueTarget
)

func validateNoDefault(field *protogen.Field, validation *pboptions.Options) string {
	condition := ""
	msg := ""

	switch field.Desc.Kind() {
	case protoreflect.BoolKind:
		condition = "!this." + field.GoName
		msg = "is required"
	case protoreflect.StringKind:
		condition = "this." + field.GoName + " == \"\""
		msg = "is required"
	case protoreflect.BytesKind:
		condition = "len(this." + field.GoName + ") == 0"
		msg = "is required"
	case protoreflect.MessageKind:
		condition = "this." + field.GoName + " == nil"
		msg = "is required"
	default:
		if field.Desc.IsList() || field.Desc.IsMap() {
			condition = "len(this." + field.GoName + ") == 0"
			msg = "is required"
		} else {
			condition = "this." + field.GoName + " == 0"
			msg = "is required"
		}
	}

	return validateField(validation, msg, condition)
}

func validateMaxMinLength(field *protogen.Field, validation *pboptions.Options) string {
	kind := field.Desc.Kind()
	if kind != protoreflect.StringKind && kind != protoreflect.BytesKind {
		compilerErrField(field, fmt.Sprintf("max_length / min_length validation only applies to string or bytes fields"))
	}
	strMax := validation.String("max_length")
	strMin := validation.String("min_length")
	goMax := getInt(field, strMax, 32)
	goMin := getInt(field, strMin, 32)

	condition := ""
	msg := ""
	elements := "characters"
	if kind == protoreflect.BytesKind {
		elements = "bytes"
	}

	if goMax != "" && goMin != "" {
		condition = fmt.Sprintf("len(this.%s) < %s || len(this.%s) > %s", field.GoName, goMin, field.GoName, goMax)
		msg = fmt.Sprintf(`"must be between %s and %s %s"`, strMin, strMax, elements)
	} else if goMax != "" {
		condition = fmt.Sprintf("len(this.%s) > %s", field.GoName, goMax)
		msg = fmt.Sprintf(`"must be at most %s %s"`, strMax, elements)
	} else if goMin != "" {
		condition = fmt.Sprintf("len(this.%s) < %s", field.GoName, goMin)
		msg = fmt.Sprintf(`"must be at least %s %s"`, strMin, elements)
	} else {
		return ""
	}

	customMsg := validation.String("msg")
	if customMsg != "" {
		msg = strconv.Quote(customMsg)
	}

	var buf bytes.Buffer
	templates.ValidateFieldTmp.Execute(&buf, struct {
		Condition string
		Msg       string
	}{condition, msg})

	return fmt.Sprintf("\t// Validate %s length\n\t{\n\t\tn := %s\n\n%s\t}\n",
		field.GoName, strconv.Quote(string(field.Desc.Name())), buf.String())
}

func validateMaxMinItems(field *protogen.Field, validation *pboptions.Options) string {
	validateCode := ""

	if maxItems := validation.String("max_items"); maxItems != "" {
		condition := fmt.Sprintf("len(this.%s) > %s", field.GoName, maxItems)
		msg := fmt.Sprintf("must have at most %s items", maxItems)
		validateCode += validateField(validation, msg, condition)
	}

	if minItems := validation.String("min_items"); minItems != "" {
		condition := fmt.Sprintf("len(this.%s) < %s", field.GoName, minItems)
		msg := fmt.Sprintf("must have at least %s items", minItems)
		validateCode += validateField(validation, msg, condition)
	}

	return validateCode
}

func validateSimpleValue(field *protogen.Field, validation *pboptions.Options,
	addPattern func(string) int, addSet func([]string, string) int, target targetType) string {

	validateCode := ""

	// Min/max validation
	if validation.String("min") != "" || validation.String("max") != "" {
		validateCode += validateMinMax(field, validation, target)
	}

	// Pattern validation
	if pattern := validation.String("pattern"); pattern != "" {
		patternIdx := addPattern(pattern)
		validateCode += validatePattern(field, validation, patternIdx, target)
	}

	// In/not_in validation
	if validation.String("in[0]") != "" {
		var values []string
		for i := 0; validation.String(fmt.Sprintf("in[%d]", i)) != ""; i++ {
			values = append(values, validation.String(fmt.Sprintf("in[%d]", i)))
		}
		setIdx := addSet(values, getFieldType(field, target))
		validateCode += validateIn(field, validation, setIdx, target)
	}

	// Well-known format validation
	for _, format := range []string{"uuid", "email", "uri", "ip", "ipv4", "ipv6"} {
		if validation.Bool(format) {
			validateCode += validateWellKnownFormat(field, validation, format, target)
		}
	}

	return validateCode
}

func validateMinMax(field *protogen.Field, validation *pboptions.Options, target targetType) string {
	validateCode := ""

	// Determine the variable name based on target
	varName := ""
	switch target {
	case fieldTarget:
		varName = "this." + field.GoName
	case itemTarget, keyTarget, valueTarget:
		varName = "v"
	}

	if min := validation.String("min"); min != "" {
		exclMin := validation.Bool("excl_min")
		op := "<"
		if exclMin {
			op = "<="
		}
		condition := fmt.Sprintf("%s %s %s", varName, op, min)
		msg := fmt.Sprintf("must be greater than %s", min)
		if exclMin {
			msg = fmt.Sprintf("must be greater than or equal to %s", min)
		}
		validateCode += validateField(validation, msg, condition)
	}

	if max := validation.String("max"); max != "" {
		exclMax := validation.Bool("excl_max")
		op := ">"
		if exclMax {
			op = ">="
		}
		condition := fmt.Sprintf("%s %s %s", varName, op, max)
		msg := fmt.Sprintf("must be less than %s", max)
		if exclMax {
			msg = fmt.Sprintf("must be less than or equal to %s", max)
		}
		validateCode += validateField(validation, msg, condition)
	}

	return validateCode
}

func validatePattern(field *protogen.Field, validation *pboptions.Options, patternIdx int, target targetType) string {
	// Determine the variable name based on target
	varName := ""
	switch target {
	case fieldTarget:
		varName = "this." + field.GoName
	case itemTarget, keyTarget, valueTarget:
		varName = "v"
	}

	condition := fmt.Sprintf("!pattern%d.MatchString(%s)", patternIdx, varName)
	msg := "must match pattern"
	return validateField(validation, msg, condition)
}

func validateIn(field *protogen.Field, validation *pboptions.Options, setIdx int, target targetType) string {
	// Determine the variable name based on target
	varName := ""
	switch target {
	case fieldTarget:
		varName = "this." + field.GoName
	case itemTarget, keyTarget, valueTarget:
		varName = "v"
	}

	condition := fmt.Sprintf("!set%d[%s]", setIdx, varName)
	msg := "must be in allowed values"
	return validateField(validation, msg, condition)
}

func validateWellKnownFormat(field *protogen.Field, validation *pboptions.Options, format string, target targetType) string {
	// Determine the variable name based on target
	varName := ""
	switch target {
	case fieldTarget:
		varName = "this." + field.GoName
	case itemTarget, keyTarget, valueTarget:
		varName = "v"
	}

	condition := ""
	msg := ""

	switch format {
	case "uuid":
		condition = fmt.Sprintf("_, err := uuid.Parse(%s); err != nil", varName)
		msg = "must be a valid UUID"
	case "email":
		condition = fmt.Sprintf("_, err := mail.ParseAddress(%s); err != nil", varName)
		msg = "must be a valid email address"
	case "uri":
		condition = fmt.Sprintf("_, err := url.ParseRequestURI(%s); err != nil", varName)
		msg = "must be a valid URI"
	case "ip":
		condition = fmt.Sprintf("ip := net.ParseIP(%s); ip == nil", varName)
		msg = "must be a valid IP address"
	case "ipv4":
		condition = fmt.Sprintf("ip := net.ParseIP(%s); ip == nil || strings.Contains(%s, \":\")", varName, varName)
		msg = "must be a valid IPv4 address"
	case "ipv6":
		condition = fmt.Sprintf("ip := net.ParseIP(%s); ip == nil || !strings.Contains(%s, \":\")", varName, varName)
		msg = "must be a valid IPv6 address"
	}

	return validateField(validation, msg, condition)
}

func getFieldType(field *protogen.Field, target targetType) string {
	// Return the Go type for the field
	switch field.Desc.Kind() {
	case protoreflect.StringKind:
		return "string"
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
	case protoreflect.BoolKind:
		return "bool"
	default:
		return "interface{}"
	}
}

func generateFieldValidation(field *protogen.Field, fieldCode, itemsCode, keysCode, valuesCode string) string {
	// Generate the complete field validation code
	code := ""

	if field.Desc.IsList() {
		// Handle repeated fields
		if fieldCode != "" {
			code += fmt.Sprintf(`
	// Validate %s
	{
		n := "%s"
		%s
	}`, field.GoName, field.Desc.Name(), fieldCode)
		}

		if itemsCode != "" {
			code += fmt.Sprintf(`
	// Validate items in %s
	for i, v := range this.%s {
		n := "%s[" + strconv.Itoa(i) + "]"
		%s
	}`, field.GoName, field.GoName, field.Desc.Name(), itemsCode)
		}
	} else if field.Desc.IsMap() {
		// Handle map fields
		if fieldCode != "" {
			code += fmt.Sprintf(`
	// Validate %s
	{
		n := "%s"
		%s
	}`, field.GoName, field.Desc.Name(), fieldCode)
		}

		if keysCode != "" || valuesCode != "" {
			// Determine if we need both k and v in the range
			if keysCode != "" && valuesCode != "" {
				code += fmt.Sprintf(`
	// Validate keys and values in %s
	for k, v := range this.%s {`, field.GoName, field.GoName)
			} else if keysCode != "" {
				code += fmt.Sprintf(`
	// Validate keys in %s
	for k := range this.%s {`, field.GoName, field.GoName)
			} else {
				code += fmt.Sprintf(`
	// Validate values in %s
	for k, v := range this.%s {`, field.GoName, field.GoName)
			}

			if keysCode != "" {
				code += fmt.Sprintf(`
		// Validate key
		{
			v := k
			n := "%s.key"
			%s
		}`, field.Desc.Name(), keysCode)
			}

			if valuesCode != "" {
				code += fmt.Sprintf(`
		// Validate value
		{
			n := "%s[" + fmt.Sprint(k) + "]"
			%s
		}`, field.Desc.Name(), valuesCode)
			}

			code += `
	}`
		}
	} else {
		// Handle singular fields
		if fieldCode != "" {
			code += fmt.Sprintf(`
	// Validate %s
	{
		n := "%s"
		%s
	}`, field.GoName, field.Desc.Name(), fieldCode)
		}

		// For message types, add recursive validation
		if field.Desc.Kind() == protoreflect.MessageKind {
			code += fmt.Sprintf(`
	// Recursively validate %s
	if this.%s != nil {
		n := "%s"
		v := this.%s
		%s
	}`, field.GoName, field.GoName, field.Desc.Name(), field.GoName, templates.ValidateMsg)
		}
	}

	return code
}

// Verify that ext proto fields match the original proto
func verifyProtoMatch(extFile *protogen.File, originalFile *protogen.File) error {
	// Map messages by name for comparison
	originalMsgs := make(map[string]*protogen.Message)
	for _, msg := range originalFile.Messages {
		originalMsgs[string(msg.Desc.Name())] = msg
	}

	// Verify each ext message matches original
	for _, extMsg := range extFile.Messages {
		// Remove _Ext suffix if present
		originalName := strings.TrimSuffix(string(extMsg.Desc.Name()), "_Ext")

		originalMsg, ok := originalMsgs[originalName]
		if !ok {
			return fmt.Errorf("ext message %s does not have corresponding original message", extMsg.Desc.Name())
		}

		// Verify fields match
		if err := verifyFieldsMatch(extMsg, originalMsg); err != nil {
			return fmt.Errorf("message %s: %v", extMsg.Desc.Name(), err)
		}
	}

	return nil
}

func verifyFieldsMatch(extMsg, originalMsg *protogen.Message) error {
	// Map original fields by name
	originalFields := make(map[string]*protogen.Field)
	for _, field := range originalMsg.Fields {
		originalFields[string(field.Desc.Name())] = field
	}

	// Verify each ext field matches original
	for _, extField := range extMsg.Fields {
		originalField, ok := originalFields[string(extField.Desc.Name())]
		if !ok {
			return fmt.Errorf("field %s not found in original message", extField.Desc.Name())
		}

		// Verify field types match
		if extField.Desc.Kind() != originalField.Desc.Kind() {
			return fmt.Errorf("field %s type mismatch: ext has %v, original has %v",
				extField.Desc.Name(), extField.Desc.Kind(), originalField.Desc.Kind())
		}

		// Verify cardinality matches (repeated, optional, etc.)
		if extField.Desc.Cardinality() != originalField.Desc.Cardinality() {
			return fmt.Errorf("field %s cardinality mismatch", extField.Desc.Name())
		}
	}

	// Verify no missing fields
	for _, originalField := range originalMsg.Fields {
		found := false
		for _, extField := range extMsg.Fields {
			if extField.Desc.Name() == originalField.Desc.Name() {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("original field %s missing in ext message", originalField.Desc.Name())
		}
	}

	return nil
}

func generate(reqData []byte) *pluginpb.CodeGeneratorResponse {
	gen, extTypes, err := util.GetPluginAndExtensions(reqData, false)
	if err != nil {
		logger.Panic(err)
	}

	// Parse options
	req := &pluginpb.CodeGeneratorRequest{}
	if err := proto.Unmarshal(reqData, req); err != nil {
		logger.Panic(err)
	}
	opts := parseOptions(req)

	// If original proto is specified, verify fields match
	if opts.OriginalProto != "" {
		// Find the original proto file
		var originalFile *protogen.File
		for _, f := range gen.Files {
			if strings.Contains(f.Desc.Path(), opts.OriginalProto) {
				originalFile = f
				break
			}
		}

		if originalFile != nil {
			for _, f := range gen.Files {
				if f.Generate {
					if err := verifyProtoMatch(f, originalFile); err != nil {
						logger.Panicf("Proto verification failed: %v", err)
					}
				}
			}
		}
	}

	for _, f := range gen.Files {
		// Skip files that don't need generation
		if !f.Generate {
			continue
		}
		generateFile(gen, f, extTypes, opts, gen.Files)
	}

	return gen.Response()
}

func main() {
	reqData := util.ReadRequest()
	resp := generate(reqData)
	util.WriteResponse(resp)
}
