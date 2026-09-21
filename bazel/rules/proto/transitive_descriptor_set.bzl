"""Merges the transitive descriptor sets of proto_library targets.

Bazel's native proto_library only writes a descriptor set for the .proto
files declared directly on the target (the `<name>-descriptor-set.proto.bin`
default output); the full transitive set is available only through the
`ProtoInfo.transitive_descriptor_sets` provider, which holds one direct-only
FileDescriptorSet per proto_library node in the dependency graph. This rule
merges those into a single self-contained FileDescriptorSet — equivalent to
running protoc with --include_imports on the same target.
"""

load("@rules_proto//proto:defs.bzl", "ProtoInfo")

def _transitive_descriptor_set_impl(ctx):
    descriptor_sets = depset(transitive = [dep[ProtoInfo].transitive_descriptor_sets for dep in ctx.attr.deps])
    output = ctx.actions.declare_file(ctx.label.name + ".proto.bin")

    args = ctx.actions.args()
    args.add("-out", output)
    args.add_all(descriptor_sets)

    ctx.actions.run(
        executable = ctx.executable._merger,
        arguments = [args],
        inputs = descriptor_sets,
        outputs = [output],
        mnemonic = "MergeProtoDescriptorSets",
        progress_message = "Merging transitive descriptor sets for %{label}",
    )

    return [DefaultInfo(files = depset([output]))]

transitive_descriptor_set = rule(
    implementation = _transitive_descriptor_set_impl,
    attrs = {
        "deps": attr.label_list(
            mandatory = True,
            allow_empty = False,
            providers = [ProtoInfo],
        ),
        "_merger": attr.label(
            cfg = "exec",
            executable = True,
            default = Label("//go/cmd/mergedescriptorset"),
        ),
    },
    doc = """Produces a single FileDescriptorSet containing every proto file
transitively imported by `deps`, equivalent to running protoc with
--include_imports on the same target(s).

Args:
  deps: proto_library targets whose transitive descriptor sets should be merged.
""",
)
