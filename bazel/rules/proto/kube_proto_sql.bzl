"""
Generate SQL schema from proto_library rules.

Bazel rule that uses protoc plugin protoc-gen-sql to generate
SQL schema file from protocol buffer schema for Michelangelo API object types.
"""

load("@rules_proto//proto:defs.bzl", "ProtoInfo")
load(
    ":protobuf.bzl",
    "declare_out_files",
    "get_include_directory",
    "get_out_dir",
    "get_proto_arguments",
    "includes_from_deps",
    "protos_from_context",
)

_GENERATED_KUBEYAML_FORMAT = "{}.pb.sql"

def _proto_gen_impl(context):
    protos = protos_from_context(context)
    includes = includes_from_deps(context.attr.deps)
    out_files = declare_out_files(protos, context, _GENERATED_KUBEYAML_FORMAT)
    tools = [context.executable._protoc]
    tools.append(context.executable._sql_plugin)

    out_dir = get_out_dir(protos, context)
    arguments = ([
        "--plugin=protoc-gen-sql={}".format(context.executable._sql_plugin.path),
        "--sql_out=:{}".format(out_dir.path),
    ] + [
        "--proto_path={}".format(get_include_directory(i))
        for i in includes
    ] + [
        "--proto_path={}".format(context.genfiles_dir.path),
    ])

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

kube_proto_sql = rule(
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
        "_sql_plugin": attr.label(
            cfg = "host",
            allow_files = True,
            executable = True,
            default = Label("//go/cmd/kubeproto/protoc-gen-sql:protoc-gen-sql"),
        ),
    },
    implementation = _proto_gen_impl,
    doc = """Generates Kubernetes CRD SQL schmea from Protocol Buffer.

Args:
  deps: a proto_library target
""",
)
