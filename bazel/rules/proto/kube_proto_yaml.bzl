"""Generate k8s CRD yaml files from proto_library target.

Bazel rule that uses protoc plugin protoc-gen-kubeyaml to generate Kubernetes CRD (Custom Resource Definition)
yaml files from protocol buffer definitions.

This tool is for testing and debugging purposes only. The same yaml schemas are embedded in the go code generated
by protoc-gen-kubeproto (go_kubeproto compiler). The CRD schemas are registered / updated automatically by go code
in Michelangelo API server. Therefore, Michelangleo users never need to manually generate and apply CRD yaml files.
"""

load("@rules_proto//proto:defs.bzl", "ProtoInfo")
load(
    "@grpc//bazel:protobuf.bzl",
    "declare_out_files",
    "get_include_directory",
    "get_out_dir",
    "get_proto_arguments",
    "includes_from_deps",
    "protos_from_context",
)

_GENERATED_KUBEYAML_FORMAT = "{}.pb.yaml"

def _proto_gen_impl(context):
    protos = protos_from_context(context)
    includes = includes_from_deps(context.attr.deps)
    out_files = declare_out_files(protos, context, _GENERATED_KUBEYAML_FORMAT)
    tools = [context.executable._protoc]
    tools.append(context.executable._kubeyaml_plugin)

    out_dir = get_out_dir(protos, context)
    arguments = ([
        "--plugin=protoc-gen-kubeyaml={}".format(context.executable._kubeyaml_plugin.path),
        "--kubeyaml_out=:{}".format(out_dir.path),
    ] + [
        "--proto_path={}".format(get_include_directory(i))
        for i in includes
    ] + [
        "--proto_path={}".format(context.genfiles_dir.path),
    ])
    arguments = depset(arguments).to_list()

    arguments += get_proto_arguments(protos, context.genfiles_dir.path)

    context.actions.run(
        inputs = protos + includes,
        tools = tools,
        outputs = out_files,
        executable = context.executable._protoc,
        arguments = arguments,
        mnemonic = "ProtocInvocation",
    )

    return [
        DefaultInfo(files = depset(direct = out_files)),
    ]

kube_proto_yaml = rule(
    attrs = {
        "deps": attr.label_list(
            mandatory = True,
            allow_empty = False,
            providers = [ProtoInfo],
        ),
        "_protoc": attr.label(
            cfg = "host",
            executable = True,
            default = Label("@com_google_protobuf//:protoc"),
            providers = ["files_to_run"],
        ),
        "_kubeyaml_plugin": attr.label(
            cfg = "host",
            allow_files = True,
            executable = True,
            default = Label("//go/cmd/kubeproto/protoc-gen-kubeyaml:protoc-gen-kubeyaml"),
        ),
    },
    implementation = _proto_gen_impl,
    doc = """Generates Kubernetes CRD yaml from Protocol Buffer.

Args:
  deps: a proto_library target
""",
)
