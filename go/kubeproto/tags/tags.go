package tags

import (
	"fmt"
	"regexp"
	"strings"
)

var jsonTagPattern = regexp.MustCompile("json:\"([^\"]*)\"")
var pbTagPattern = regexp.MustCompile("protobuf:\"([^\"]*)\"")

// JSONTag parsed json tag of a go field
type JSONTag struct {
	Name    string
	Options []string
}

// PBTag parsed protobuf tag of a go field
type PBTag struct {
	Options []string
}

// GetJSONTag parse json tag
func GetJSONTag(tag string) *JSONTag {
	match := jsonTagPattern.FindStringSubmatch(tag)
	if len(match) <= 1 {
		return nil
	}

	options := strings.Split(match[1], ",")
	return &JSONTag{Name: options[0], Options: options[1:]}
}

// SetJSONTag generate json tag string from jsonTag, replace the existing json tag in tag
func SetJSONTag(tag *string, jsonTag *JSONTag) {
	newTag := fmt.Sprintf(`json:"%v"`, jsonTag.String())
	*tag = string(jsonTagPattern.ReplaceAll([]byte(*tag), []byte(newTag)))
}

// generate json tag string
func (t *JSONTag) String() string {
	if t.Options != nil && len(t.Options) > 0 {
		return t.Name + "," + strings.Join(t.Options, ",")
	}
	return t.Name
}

// GetPBTag parse protobuf tag
func GetPBTag(tag string) *PBTag {
	match := pbTagPattern.FindStringSubmatch(tag)
	if len(match) <= 1 {
		return nil
	}

	return &PBTag{strings.Split(match[1], ",")}
}

// GetJSONName return json field name
func (t *PBTag) GetJSONName() string {
	jsonName := ""
	for _, option := range t.Options {
		if len(jsonName) == 0 && strings.HasPrefix(option, "name=") {
			jsonName = strings.TrimPrefix(option, "name=")
		}
		if strings.HasPrefix(option, "json=") {
			jsonName = strings.TrimPrefix(option, "json=")
		}
	}
	return jsonName
}
