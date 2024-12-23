package tags

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTags(t *testing.T) {
	assert.True(t, GetJSONTag(``) == nil)
	assert.True(t, GetJSONTag(`protobuf:"bytes,2,rep,name=versions"`) == nil)
	assert.Equal(t, "versions,omitempty", GetJSONTag(`protobuf:"bytes,2,rep,name=versions" json:"versions,omitempty"`).String())
	assert.Equal(t, "versions", GetJSONTag(`protobuf:"bytes,2,rep,name=versions" json:"versions,omitempty"`).Name)
	assert.Equal(t, "name", GetJSONTag(`protobuf:"bytes,1,opt,name=name" json:"name"`).Name)
	assert.Equal(t, "", GetJSONTag(`protobuf:"bytes,1,opt,name=type_meta,json=typeMeta,proto3,embedded=type_meta" json:",inline"`).Name)
	assert.True(t, GetPBTag(``) == nil)
	assert.Equal(t, "name", GetPBTag(`protobuf:"bytes,1,opt,name=name" json:"name"`).GetJSONName())
	assert.Equal(t, "typeMeta", GetPBTag(`protobuf:"bytes,1,opt,name=type_meta,json=typeMeta,proto3,embedded=type_meta" json:",inline"`).GetJSONName())

	tag := `protobuf:"bytes,2,rep,name=versions" json:"versions,omitempty"`
	jsonTag := GetJSONTag(tag)
	jsonTag.Name = "test"
	SetJSONTag(&tag, jsonTag)
	assert.Equal(t, `protobuf:"bytes,2,rep,name=versions" json:"test,omitempty"`, tag)
}
