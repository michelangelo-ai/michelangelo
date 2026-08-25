// Command mergedescriptorset merges multiple serialized FileDescriptorSet
// messages into one, deduplicating files by name.
//
// Bazel's proto_library only writes a descriptor set for the .proto files
// declared directly on the target; the ProtoInfo.transitive_descriptor_sets
// provider exposes one such direct-only descriptor set per proto_library
// node in the dependency graph. This tool combines them into a single
// self-contained FileDescriptorSet, equivalent to what protoc's own
// --include_imports flag produces.
package main

import (
	"flag"
	"fmt"
	"os"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

func main() {
	out := flag.String("out", "", "output path for the merged FileDescriptorSet")
	flag.Parse()

	if *out == "" || flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: mergedescriptorset -out=<path> <input.proto.bin>...")
		os.Exit(1)
	}

	if err := run(*out, flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(out string, inputs []string) error {
	merged := &descriptorpb.FileDescriptorSet{}
	seen := make(map[string]bool)

	for _, path := range inputs {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		set := &descriptorpb.FileDescriptorSet{}
		if err := proto.Unmarshal(data, set); err != nil {
			return fmt.Errorf("unmarshaling %s: %w", path, err)
		}

		for _, file := range set.GetFile() {
			name := file.GetName()
			if seen[name] {
				continue
			}
			seen[name] = true
			merged.File = append(merged.File, file)
		}
	}

	data, err := proto.Marshal(merged)
	if err != nil {
		return fmt.Errorf("marshaling merged descriptor set: %w", err)
	}

	if err := os.WriteFile(out, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", out, err)
	}

	return nil
}
