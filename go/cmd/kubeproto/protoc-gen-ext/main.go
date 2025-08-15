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
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

var logger = log.New(os.Stderr, "", 0)

func generateFile(gen *protogen.Plugin, file *protogen.File, extTypes *protoregistry.Types, allFiles []*protogen.File) *protogen.GeneratedFile {
	// Check if this file has ext_original_proto option
	fileOptions := file.Desc.Options().(*descriptorpb.FileOptions)
	pbFileOptions, err := pboptions.ReadOptions(extTypes, fileOptions)
	if err != nil {
		logger.Printf("Error reading file options: %v", err)
		return nil
	}

	originalProtoPath := pbFileOptions.String("ext_original_proto")
	if originalProtoPath == "" {
		logger.Printf("No ext_original_proto option found in %s", file.Desc.Path())
		return nil
	}

	// Find the original proto file
	var originalFile *protogen.File
	for _, f := range allFiles {
		// Try exact match first
		if f.Desc.Path() == originalProtoPath {
			originalFile = f
			break
		}
		// Try match with michelangelo/ prefix stripped
		if strings.Contains(f.Desc.Path(), strings.TrimPrefix(originalProtoPath, "michelangelo/")) {
			originalFile = f
			break
		}
		// Try matching just the filename
		if strings.HasSuffix(f.Desc.Path(), strings.TrimPrefix(originalProtoPath, "michelangelo/")) {
			originalFile = f
			break
		}
	}

	if originalFile == nil {
		logger.Printf("Warning: Could not find original proto file: %s for field verification", originalProtoPath)
		logger.Printf("Proceeding without field verification. Available proto files:")
		for _, f := range allFiles {
			logger.Printf("  %s", f.Desc.Path())
		}
		// Continue without verification for now
	} else {
		// Verify fields match before generation
		if err := verifyProtoMatch(file, originalFile); err != nil {
			logger.Panicf("Proto verification failed for %s: %v", file.Desc.Path(), err)
		}
	}

	// Generate .ext.go file
	filename := file.GeneratedFilenamePrefix + ".ext.go"
	g := gen.NewGeneratedFile(filename, file.GoImportPath)

	// Write file header
	header := fmt.Sprintf(templates.FileHeader, "protoc-gen-ext", file.GoPackageName)
	g.P(header)
	g.P()

	// Global counters for patterns and sets
	globalPatternCounter := 0
	globalSetCounter := 0

	// Collect all patterns, sets, and validation functions
	allPatterns := []string{}
	allSets := []string{}
	allValidationFuncs := []string{}

	// First pass: collect all patterns, sets, and validation code
	for _, msg := range file.Messages {
		validateCode, patterns, sets, err := generateValidationCode(msg, extTypes, &globalPatternCounter, &globalSetCounter)
		if err != nil {
			logger.Panicf("Error generating validation for %s: %v", msg.GoIdent.GoName, err)
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

		// Import the original package to access its Register functions
		if originalFile != nil {

			for _, msg := range file.Messages {
				// Check if message has validation
				tempPatternCounter := 0
				tempSetCounter := 0
				validateCode, _, _, _ := generateValidationCode(msg, extTypes, &tempPatternCounter, &tempSetCounter)
				if validateCode != "" {
					// Use the message name directly (no _Ext suffix to strip)
					typeName := msg.GoIdent.GoName
					
					g.P(fmt.Sprintf(`	// Register validation for %s`, msg.GoIdent.GoName))
					g.P(fmt.Sprintf(`	ValidationRegistry["%s"] = func(obj interface{}, prefix string) error {`, msg.GoIdent.GoName))
					g.P(fmt.Sprintf(`		if msg, ok := obj.(*%s); ok {`, msg.GoIdent.GoName))
					g.P(`			return msg.Validate(prefix)`)
					g.P(`		}`)
					g.P(`		return nil`)
					g.P(`	}`)
					g.P()
					
					// Call the original validation register function
					g.P(`	// Register ext validation with original proto validation system`)
					registerFuncName := g.QualifiedGoIdent(originalFile.GoImportPath.Ident("Register" + typeName + "ValidateExt"))
					g.P(fmt.Sprintf(`	%s(func(orig *%s, prefix string) error {`, registerFuncName, g.QualifiedGoIdent(originalFile.GoImportPath.Ident(typeName))))
					g.P(`		// Create ext version with data from original`)
					g.P(fmt.Sprintf(`		extObj := &%s{}`, msg.GoIdent.GoName))
					g.P(`		// Copy fields from original to ext`)
					
					// Generate field mapping based on the ext message's fields
					for _, field := range msg.Fields {
						fieldName := field.GoName
						if field.Desc.IsList() {
							if field.Desc.Message() != nil {
								// For repeated message fields, copy each item
								g.P(fmt.Sprintf(`		for _, item := range orig.%s {`, fieldName))
								g.P(fmt.Sprintf(`			extItem := &%s{`, field.Message.GoIdent.GoName))
								// Copy fields from original item to ext item
								for _, subField := range field.Message.Fields {
									if subField.Desc.Enum() != nil {
										g.P(fmt.Sprintf(`				%s: %s(item.%s),`, subField.GoName, subField.Enum.GoIdent.GoName, subField.GoName))
									} else {
										g.P(fmt.Sprintf(`				%s: item.%s,`, subField.GoName, subField.GoName))
									}
								}
								g.P(`			}`)
								g.P(fmt.Sprintf(`			extObj.%s = append(extObj.%s, extItem)`, fieldName, fieldName))
								g.P(`		}`)
							} else {
								// For primitive repeated fields, direct copy
								g.P(fmt.Sprintf(`		extObj.%s = orig.%s`, fieldName, fieldName))
							}
						} else if field.Desc.Message() != nil {
							// For single message fields, copy if not nil
							g.P(fmt.Sprintf(`		if orig.%s != nil {`, fieldName))
							g.P(fmt.Sprintf(`			extObj.%s = &%s{`, fieldName, field.Message.GoIdent.GoName))
							for _, subField := range field.Message.Fields {
								if subField.Desc.Enum() != nil {
									g.P(fmt.Sprintf(`				%s: %s(orig.%s.%s),`, subField.GoName, subField.Enum.GoIdent.GoName, fieldName, subField.GoName))
								} else {
									g.P(fmt.Sprintf(`				%s: orig.%s.%s,`, subField.GoName, fieldName, subField.GoName))
								}
							}
							g.P(`			}`)
							g.P(`		}`)
						} else if field.Desc.Enum() != nil {
							// For enum fields, cast to ext enum type
							g.P(fmt.Sprintf(`		extObj.%s = %s(orig.%s)`, fieldName, field.Enum.GoIdent.GoName, fieldName))
						} else {
							// For primitive fields, direct copy
							g.P(fmt.Sprintf(`		extObj.%s = orig.%s`, fieldName, fieldName))
						}
					}
					
					g.P(`		return extObj.Validate(prefix)`)
					g.P(`	})`)
				}
			}
		} else {
			// If no original file found, just register ext validations
			for _, msg := range file.Messages {
				// Check if message has validation
				tempPatternCounter := 0
				tempSetCounter := 0
				validateCode, _, _, _ := generateValidationCode(msg, extTypes, &tempPatternCounter, &tempSetCounter)
				if validateCode != "" {
					g.P(fmt.Sprintf(`	// Register validation for %s`, msg.GoIdent.GoName))
					g.P(fmt.Sprintf(`	ValidationRegistry["%s"] = func(obj interface{}, prefix string) error {`, msg.GoIdent.GoName))
					g.P(fmt.Sprintf(`		if msg, ok := obj.(*%s); ok {`, msg.GoIdent.GoName))
					g.P(`			return msg.Validate(prefix)`)
					g.P(`		}`)
					g.P(`		return nil`)
					g.P(`	}`)
				}
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

// verifyProtoMatch verifies that ext proto fields match original proto fields
func verifyProtoMatch(extFile *protogen.File, originalFile *protogen.File) error {
	// Create maps for quick lookup
	originalMessages := make(map[string]*protogen.Message)
	for _, msg := range originalFile.Messages {
		originalMessages[string(msg.Desc.Name())] = msg
	}

	// Check each ext message
	for _, extMsg := range extFile.Messages {
		// Use the message name directly (ext and original should have same name in different packages)
		messageName := string(extMsg.Desc.Name())
		
		originalMsg, exists := originalMessages[messageName]
		if !exists {
			return fmt.Errorf("ext message %s does not have corresponding original message %s", extMsg.Desc.Name(), messageName)
		}

		// Verify fields match
		if err := verifyMessageFields(extMsg, originalMsg); err != nil {
			return fmt.Errorf("field mismatch in %s: %v", extMsg.Desc.Name(), err)
		}
	}

	return nil
}

// verifyMessageFields verifies that ext message fields match original message fields
func verifyMessageFields(extMsg *protogen.Message, originalMsg *protogen.Message) error {
	// Create field maps
	originalFields := make(map[string]*protogen.Field)
	for _, field := range originalMsg.Fields {
		originalFields[string(field.Desc.Name())] = field
	}

	for _, extField := range extMsg.Fields {
		fieldName := string(extField.Desc.Name())
		originalField, exists := originalFields[fieldName]
		if !exists {
			return fmt.Errorf("field %s not found in original message", fieldName)
		}

		// Check field types match
		if extField.Desc.Kind() != originalField.Desc.Kind() {
			return fmt.Errorf("field %s type mismatch: ext=%v, original=%v", 
				fieldName, extField.Desc.Kind(), originalField.Desc.Kind())
		}

		// Check cardinality (repeated, optional, etc.)
		if extField.Desc.IsList() != originalField.Desc.IsList() {
			return fmt.Errorf("field %s list mismatch: ext=%v, original=%v", 
				fieldName, extField.Desc.IsList(), originalField.Desc.IsList())
		}

		if extField.Desc.IsMap() != originalField.Desc.IsMap() {
			return fmt.Errorf("field %s map mismatch: ext=%v, original=%v", 
				fieldName, extField.Desc.IsMap(), originalField.Desc.IsMap())
		}
	}

	return nil
}

// generateValidationCode generates validation code for a message
func generateValidationCode(msg *protogen.Message, extTypes *protoregistry.Types, patternCounter *int, setCounter *int) (string, []string, []string, error) {
	var patterns []string
	var sets []string
	validateCode := ""

	// Process message options
	pbOptions := msg.Desc.Options().(*descriptorpb.MessageOptions)
	msgOptions, err := pboptions.ReadOptions(extTypes, pbOptions)
	if err != nil {
		return "", nil, nil, err
	}

	noDefault := msgOptions.Bool("no_default")

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
						if typ == "string" {
							setStr += fmt.Sprintf("%s: true, ", strconv.Quote(v))
						} else if strings.Contains(typ, "int") || strings.Contains(typ, "uint") {
							setStr += fmt.Sprintf("%s: true, ", v)
						} else if typ == "float32" || typ == "float64" {
							setStr += fmt.Sprintf("%s: true, ", v)
						} else if typ == "bool" {
							setStr += fmt.Sprintf("%s: true, ", v)
						} else {
							// Enum or other type - cast from int
							setStr += fmt.Sprintf("%s(%s): true, ", typ, v)
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

		// Add message validation for nested messages (like validation compiler)
		if field.Desc.Kind() == protoreflect.MessageKind && !field.Desc.IsMap() && !field.Desc.IsList() {
			fieldValidateCode += templates.ValidateMsg
		}
		if field.Desc.IsList() && field.Desc.Kind() == protoreflect.MessageKind {
			itemsValidateCode += templates.ValidateMsg
		}
		if field.Desc.IsMap() && field.Desc.MapValue().Kind() == protoreflect.MessageKind {
			valuesValidateCode += templates.ValidateMsg
		}

		// Add field validation code only if there's actual validation
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

// Helper functions for validation (simplified versions)
func validateNoDefault(field *protogen.Field, validation *pboptions.Options) string {
	condition := ""
	msg := "is required"

	switch field.Desc.Kind() {
	case protoreflect.BoolKind:
		condition = "!v"
	case protoreflect.StringKind:
		condition = "v == \"\""
	case protoreflect.BytesKind:
		condition = "len(v) == 0"
	case protoreflect.MessageKind:
		condition = "v == nil"
	default:
		if field.Desc.IsList() || field.Desc.IsMap() {
			condition = "len(v) == 0"
		} else {
			condition = "v == 0"
		}
	}

	return validateFieldWithVar(validation, msg, condition)
}

func validateFieldDirect(validation *pboptions.Options, errMsg string, condition string, field *protogen.Field) string {
	if validation != nil && validation.String("msg") != "" {
		errMsg = validation.String("msg")
	}
	var buf bytes.Buffer
	templates.ValidateFieldTmp.Execute(&buf, struct {
		Condition string
		Msg       string
	}{strings.ReplaceAll(condition, "v", "this."+field.GoName), strconv.Quote(errMsg)})
	return buf.String()
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

func validateFieldWithVar(validation *pboptions.Options, errMsg string, condition string) string {
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

func validateMaxMinItems(field *protogen.Field, validation *pboptions.Options) string {
	validateCode := ""

	if maxItems := validation.String("max_items"); maxItems != "" {
		condition := fmt.Sprintf("len(v) > %s", maxItems)
		msg := fmt.Sprintf("must have at most %s items", maxItems)
		validateCode += validateField(validation, msg, condition)
	}

	if minItems := validation.String("min_items"); minItems != "" {
		condition := fmt.Sprintf("len(v) < %s", minItems)
		msg := fmt.Sprintf("must have at least %s items", minItems)
		validateCode += validateField(validation, msg, condition)
	}

	return validateCode
}

func validateMaxMinLength(field *protogen.Field, validation *pboptions.Options) string {
	kind := field.Desc.Kind()
	if kind != protoreflect.StringKind && kind != protoreflect.BytesKind {
		return ""
	}

	validateCode := ""
	elements := "characters"
	if kind == protoreflect.BytesKind {
		elements = "bytes"
	}

	if maxLen := validation.String("max_length"); maxLen != "" {
		condition := fmt.Sprintf("len(v) > %s", maxLen)
		msg := fmt.Sprintf("must be at most %s %s", maxLen, elements)
		validateCode += validateFieldWithVar(validation, msg, condition)
	}

	if minLen := validation.String("min_length"); minLen != "" {
		condition := fmt.Sprintf("len(v) < %s", minLen)
		msg := fmt.Sprintf("must be at least %s %s", minLen, elements)
		validateCode += validateFieldWithVar(validation, msg, condition)
	}

	return validateCode
}

// Target types for validation
type targetType int

const (
	fieldTarget targetType = iota
	itemTarget
	keyTarget
	valueTarget
)

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

	return validateCode
}

func validateMinMax(field *protogen.Field, validation *pboptions.Options, target targetType) string {
	validateCode := ""
	varName := getVarName(field, target)
	
	min := validation.String("min")
	max := validation.String("max")

	if min != "" {
		condition := fmt.Sprintf("%s < %s", varName, min)
		msg := fmt.Sprintf("must be greater than %s", min)
		validateCode += validateFieldWithVar(validation, msg, condition)
	}

	if max != "" {
		condition := fmt.Sprintf("%s > %s", varName, max)
		msg := fmt.Sprintf("must be less than %s", max)
		validateCode += validateFieldWithVar(validation, msg, condition)
	}

	return validateCode
}

func validatePattern(field *protogen.Field, validation *pboptions.Options, patternIdx int, target targetType) string {
	varName := getVarName(field, target)
	condition := fmt.Sprintf("!pattern%d.MatchString(%s)", patternIdx, varName)
	msg := "must match pattern"
	return validateFieldWithVar(validation, msg, condition)
}

func validateIn(field *protogen.Field, validation *pboptions.Options, setIdx int, target targetType) string {
	varName := getVarName(field, target)
	condition := fmt.Sprintf("!set%d[%s]", setIdx, varName)
	msg := "must be in allowed values"
	return validateFieldWithVar(validation, msg, condition)
}

func getVarName(field *protogen.Field, target targetType) string {
	switch target {
	case fieldTarget:
		return "v"
	case itemTarget, keyTarget, valueTarget:
		return "v"
	}
	return "v"
}

func getFieldType(field *protogen.Field, target targetType) string {
	var kind protoreflect.Kind
	
	switch target {
	case fieldTarget:
		if field.Desc.IsMap() || field.Desc.IsList() {
			return "string" // fallback
		}
		kind = field.Desc.Kind()
	case itemTarget:
		kind = field.Desc.Kind()
	case keyTarget:
		kind = field.Desc.MapKey().Kind()
	case valueTarget:
		kind = field.Desc.MapValue().Kind()
	}
	
	switch kind {
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
	case protoreflect.EnumKind:
		return field.Enum.GoIdent.GoName
	default:
		return "string" // fallback
	}
}

func generateFieldValidation(field *protogen.Field, fieldValidateCode, itemsValidateCode, keysValidateCode, valuesValidateCode string) string {
	validateCode := ""
	
	// Only generate validation block if there's actual validation code
	if fieldValidateCode == "" && itemsValidateCode == "" && keysValidateCode == "" && valuesValidateCode == "" {
		return ""
	}
	
	// Handle oneof fields using the same pattern as validation compiler
	if field.Oneof != nil {
		validateCode += fmt.Sprintf("\n\tif f, ok := this.%s.(*%s); ok {"+
			"\n\t\tv := f.%s", field.Oneof.GoName, field.GoIdent.GoName, field.GoName)
	} else {
		if field.Desc.Kind() == protoreflect.MessageKind {
			validateCode += fmt.Sprintf("\n\t{\n\t\tv := this.Get%s()", field.GoName)
		} else {
			validateCode += fmt.Sprintf("\n\t{\n\t\tv := this.%s", field.GoName)
		}
	}

	if fieldValidateCode != "" {
		validateCode += fmt.Sprintf("\n\t\tn := `%s`", field.Desc.TextName())
		validateCode += fieldValidateCode
	}

	// Add items/keys/values validation if needed
	if itemsValidateCode != "" {
		validateCode += fmt.Sprintf("\n\t\tfor i, v := range v {\n\t\t\tn := `%s[` + strconv.Itoa(i) + `]`\n%s\n\t\t}",
			field.Desc.TextName(), indent(itemsValidateCode))
	}

	if keysValidateCode != "" {
		validateCode += fmt.Sprintf("\n\t\tfor v := range v {\n\t\t\tn := `%s key`\n%s\n\t\t}",
			field.Desc.TextName(), indent(keysValidateCode))
	}

	if valuesValidateCode != "" {
		validateCode += fmt.Sprintf("\n\t\tfor k, v := range v {\n\t\t\tn := fmt.Sprintf(`%s[%%v]`, k)\n%s\n\t\t}",
			field.Desc.TextName(), indent(valuesValidateCode))
	}

	validateCode += "\n\t}"
	
	return validateCode
}

func indent(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = "\t" + line
		}
	}
	return strings.Join(lines, "\n")
}

// findMessageByName finds a message by name in the given proto file
func findMessageByName(file *protogen.File, name string) *protogen.Message {
	for _, msg := range file.Messages {
		if msg.GoIdent.GoName == name {
			return msg
		}
	}
	return nil
}

func generate(reqData []byte) {
	gen, extTypes, err := util.GetPluginAndExtensions(reqData, false)
	if err != nil {
		logger.Panic(err)
	}

	// Only generate files for ext protos (filter happens in generateFile)
	for _, f := range gen.Files {
		if !f.Generate {
			continue
		}
		generateFile(gen, f, extTypes, gen.Files)
	}

	util.WriteResponse(gen.Response())
}

func main() {
	reqData := util.ReadRequest()
	generate(reqData)
}