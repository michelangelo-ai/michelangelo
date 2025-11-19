package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/michelangelo-ai/michelangelo/tools/proto-patcher/config"
	"github.com/michelangelo-ai/michelangelo/tools/proto-patcher/generator"
	"github.com/michelangelo-ai/michelangelo/tools/proto-patcher/parser"
	"github.com/michelangelo-ai/michelangelo/tools/proto-patcher/patcher"
)

var (
	baseProtoDir   = flag.String("base_proto_dir", "", "Directory containing base proto files")
	extProtoDir    = flag.String("ext_proto_dir", "", "Directory containing extension proto files")
	outputDir      = flag.String("output_dir", "", "Output directory for patched protos")
	configFile     = flag.String("config", "", "Path to patch configuration YAML file")
	generateConfig = flag.Bool("generate_config", false, "Generate config from extension files")
	importPaths    = flag.String("import_paths", "", "Comma-separated list of import paths for proto parsing")
	fieldPrefix    = flag.String("field_prefix", "EXT_", "Prefix for extension fields")
	tagStart       = flag.Int("tag_start", 999, "Starting tag number for extension fields")
	verbose        = flag.Bool("verbose", false, "Enable verbose logging")
)

func main() {
	flag.Parse()

	if err := run(); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func run() error {
	// Validate required flags
	if *baseProtoDir == "" {
		return fmt.Errorf("--base_proto_dir is required")
	}
	if *extProtoDir == "" {
		return fmt.Errorf("--ext_proto_dir is required")
	}
	if *outputDir == "" {
		return fmt.Errorf("--output_dir is required")
	}

	// Parse import paths
	var importPathList []string
	if *importPaths != "" {
		importPathList = strings.Split(*importPaths, ",")
	}
	importPathList = append(importPathList, *baseProtoDir, *extProtoDir)

	if *verbose {
		log.Printf("Base proto dir: %s", *baseProtoDir)
		log.Printf("Extension proto dir: %s", *extProtoDir)
		log.Printf("Output dir: %s", *outputDir)
		log.Printf("Import paths: %v", importPathList)
	}

	// Load or generate configuration
	var cfg *patcher.Config
	var err error

	if *configFile != "" {
		if *verbose {
			log.Printf("Loading config from: %s", *configFile)
		}
		cfg, err = config.LoadConfig(*configFile)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
	} else if *generateConfig {
		if *verbose {
			log.Println("Generating config from extension files")
		}
		extFiles, extErr := findProtoFiles(*extProtoDir)
		if extErr != nil {
			return fmt.Errorf("failed to find extension protos: %w", extErr)
		}
		cfg, err = config.GenerateConfig(extFiles, *fieldPrefix, *tagStart)
		if err != nil {
			return fmt.Errorf("failed to generate config: %w", err)
		}
	} else {
		return fmt.Errorf("either --config or --generate_config must be specified")
	}

	// Find proto files
	baseFiles, err := findProtoFiles(*baseProtoDir)
	if err != nil {
		return fmt.Errorf("failed to find base protos: %w", err)
	}

	extFiles, err := findProtoFiles(*extProtoDir)
	if err != nil {
		return fmt.Errorf("failed to find extension protos: %w", err)
	}

	if *verbose {
		log.Printf("Found %d base proto files", len(baseFiles))
		log.Printf("Found %d extension proto files", len(extFiles))
	}

	// Parse proto files
	p := parser.NewParser(importPathList)

	if *verbose {
		log.Println("Parsing base protos...")
	}
	baseProtos, err := p.ParseFiles(baseFiles)
	if err != nil {
		return fmt.Errorf("failed to parse base protos: %w", err)
	}

	if *verbose {
		log.Println("Parsing extension protos...")
	}
	extProtos, err := p.ParseFiles(extFiles)
	if err != nil {
		return fmt.Errorf("failed to parse extension protos: %w", err)
	}

	// Apply patches
	if *verbose {
		log.Println("Applying patches...")
	}
	patcherInst := patcher.NewPatcher(cfg)
	patchedProtos, err := patcherInst.Patch(baseProtos, extProtos)
	if err != nil {
		return fmt.Errorf("failed to apply patches: %w", err)
	}

	if *verbose {
		log.Printf("Generated %d patched proto files", len(patchedProtos))
	}

	// Generate output files
	if *verbose {
		log.Println("Generating output files...")
	}
	gen := generator.NewGenerator()
	outputs, err := gen.GenerateAll(patchedProtos)
	if err != nil {
		return fmt.Errorf("failed to generate outputs: %w", err)
	}

	// Create output directory
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Write output files
	for filename, content := range outputs {
		outputPath := filepath.Join(*outputDir, filename)
		if *verbose {
			log.Printf("Writing: %s", outputPath)
		}
		if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", outputPath, err)
		}
	}

	if *verbose {
		log.Println("Patching completed successfully!")
	}

	return nil
}

// findProtoFiles recursively finds all .proto files in a directory
func findProtoFiles(dir string) ([]string, error) {
	var protoFiles []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".proto") {
			protoFiles = append(protoFiles, path)
		}
		return nil
	})

	return protoFiles, err
}
