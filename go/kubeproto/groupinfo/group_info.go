package groupinfo

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/michelangelo-ai/michelangelo/go/kubeproto/pboptions"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoregistry"
)

var logger = log.New(os.Stderr, "", 0)

// GroupInfo contains the information of a CRD group from groupversion_info.proto file.
type GroupInfo struct {
	Name       string
	Version    string
	HubVersion string
	File       *protogen.File
}

func isGroupFile(filename string) bool {
	groupFileID := "groupversion_info"
	if len(filename) < len(groupFileID) {
		return false
	}
	return filename[:len(groupFileID)] == groupFileID
}

// Load searches groupversion_info.proto files in the input protobuf files, and loads group version info from them
// Returns a map of protobuf package to GroupInfo
func Load(gen *protogen.Plugin, extTypes *protoregistry.Types) map[string]*GroupInfo {
	gInfoMap := make(map[string]*GroupInfo)

	for _, f := range gen.Files {
		if isGroupFile(filepath.Base(f.GeneratedFilenamePrefix)) {
			options, err := pboptions.ReadOptions(extTypes, f.Proto.Options)
			if err != nil {
				logger.Panicf("Failed to read protobuf options: %v", err)
			}
			if options.Bool("has_group_info") {
				gInfo := GroupInfo{
					Name:       options.String("group_info.name"),
					Version:    options.String("group_info.version"),
					HubVersion: options.String("group_info.hub_version"),
					File:       f,
				}
				if gInfo.Name == "" || gInfo.Version == "" {
					logger.Panicln(fmt.Sprintf("Failed to derive API group version info from file: %s.%s. "+
						"Make sure both API name and version are defined in groupversion_info.proto",
						*f.Proto.Package, f.Proto.GetName()))
				}
				gInfoMap[*f.Proto.Package] = &gInfo
			}
		}
	}

	return gInfoMap
}
