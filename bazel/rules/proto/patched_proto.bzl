"""
Proto extension framework for Michelangelo.

This rule allows organizations to extend Michelangelo protos with
their own internal fields without modifying the OSS protos.

Example:
    load("@michelangelo//bazel/rules/proto:patched_proto.bzl", "patched_proto_library")

    patched_proto_library(
        name = "michelangelo_extended",
        base_protos = "@michelangelo//proto/api/v2:v2_proto",
        extension_protos = glob(["extensions/*.proto"]),
        field_prefix = "YOUR_ORG_",
        tag_start = 999,
    )
"""

load("@rules_proto//proto:defs.bzl", "proto_library")
load("@io_bazel_rules_go//proto:def.bzl", "go_proto_library")
load("@io_bazel_rules_go//go:def.bzl", "go_library")

def patched_proto_library(
        name,
        base_protos,
        extension_protos,
        extension_config = None,
        field_prefix = "EXT_",
        tag_start = 999,
        importpath = None,
        visibility = None,
        **kwargs):
    """
    Creates a patched version of Michelangelo protos with extensions.

    This macro orchestrates the entire proto patching workflow:
    1. Extracts base proto source files
    2. Generates or uses patch configuration
    3. Runs the patch compiler to merge base and extension protos
    4. Creates proto_library from patched files
    5. Generates Go code with Michelangelo compilers
    6. Creates final go_library for importing

    Args:
        name: Name of the library (e.g., "v2" or "michelangelo_extended")
        base_protos: Label to base Michelangelo proto_library target
                    (e.g., "@michelangelo//proto/api/v2:v2_proto")
        extension_protos: List of extension proto file labels
                         (e.g., glob(["extensions/*.proto"]))
        extension_config: Optional YAML config file for advanced patching
                         If not provided, auto-generates from conventions
        field_prefix: Prefix for extension fields (default: "EXT_")
        tag_start: Starting tag number for extensions (default: 999)
        importpath: Go import path for generated code
                   If not provided, uses "YOUR_ORG/michelangelo/proto/api/{name}"
        visibility: Visibility for generated targets
        **kwargs: Additional args passed to proto_library

    Outputs:
        {name}_proto: The proto_library with patched protos
        {name}_go_proto: The go_proto_library with generated Go code
        {name}: The final go_library that services depend on
    """

    # Default import path
    if not importpath:
        importpath = "YOUR_ORG/michelangelo/proto/api/" + name

    # Step 1: Extract base proto source files
    _extract_base_proto_sources(
        name = name + "_base_sources",
        base_proto = base_protos,
    )

    # Step 2: Generate patch config if not provided
    config_target = extension_config
    if not config_target:
        config_target = ":" + name + "_patch_config"
        _generate_patch_config(
            name = name + "_patch_config",
            base_sources = ":" + name + "_base_sources",
            extension_protos = extension_protos,
            field_prefix = field_prefix,
            tag_start = tag_start,
        )

    # Step 3: Run patch compiler
    _run_patch_compiler(
        name = name + "_patched_files",
        base_sources = ":" + name + "_base_sources",
        extension_protos = extension_protos,
        config = config_target,
        field_prefix = field_prefix,
        tag_start = tag_start,
    )

    # Step 4: Create proto_library from patched files
    proto_library(
        name = name + "_proto",
        srcs = [":" + name + "_patched_files"],
        strip_import_prefix = "",
        deps = [
            "@michelangelo//proto/api:api_proto",
            "@com_google_protobuf//:any_proto",
            "@com_google_protobuf//:duration_proto",
            "@com_google_protobuf//:timestamp_proto",
            "@io_k8s_apimachinery//pkg/apis/meta/v1:v1_proto",
        ] + kwargs.pop("deps", []),
        visibility = visibility,
        **kwargs
    )

    # Step 5: Generate Go code with all Michelangelo compilers
    go_proto_library(
        name = name + "_go_proto",
        compilers = [
            "@michelangelo//bazel/rules/proto:go_kubeproto",
            "@michelangelo//bazel/rules/proto:go_validation",
            "@michelangelo//bazel/rules/proto:go_yarpc",
        ],
        importpath = importpath,
        proto = ":" + name + "_proto",
        visibility = visibility,
    )

    # Step 6: Final go_library that users import
    go_library(
        name = name,
        embed = [":" + name + "_go_proto"],
        importpath = importpath,
        visibility = visibility,
    )

def _extract_base_proto_sources(name, base_proto):
    """Extracts proto source files from base proto_library target."""
    native.genrule(
        name = name,
        srcs = [base_proto],
        outs = [name + ".txt"],
        cmd = """
        # List all proto source files from the target
        echo "$(locations {target})" > $@
        """.format(target = base_proto),
    )

def _generate_patch_config(name, base_sources, extension_protos, field_prefix, tag_start):
    """Auto-generates patch configuration from proto file names and conventions."""
    native.genrule(
        name = name,
        srcs = [base_sources] + extension_protos,
        outs = [name + ".yaml"],
        tools = ["@michelangelo//tools/proto-patcher:config-generator"],
        cmd = """
        $(location @michelangelo//tools/proto-patcher:config-generator) \\
            --base_sources=$(location {base}) \\
            --ext_protos="$(locations {ext})" \\
            --field_prefix={prefix} \\
            --tag_start={tag} \\
            --output=$@
        """.format(
            base = base_sources,
            ext = " ".join(["$(location %s)" % p for p in extension_protos]),
            prefix = field_prefix,
            tag = tag_start,
        ),
    )

def _run_patch_compiler(name, base_sources, extension_protos, config, field_prefix, tag_start):
    """Runs the patch compiler to merge base and extension protos."""

    # List of expected output files
    # TODO: Make this dynamic based on input protos
    output_files = [
        "project_patched.proto",
        "deployment_patched.proto",
        "model_patched.proto",
        "pipeline_patched.proto",
        "pipeline_run_patched.proto",
        "trigger_run_patched.proto",
        "cached_output_patched.proto",
        "ray_cluster_patched.proto",
        "ray_job_patched.proto",
        "spark_job_patched.proto",
        "pod_patched.proto",
        "model_family_patched.proto",
        "cluster_patched.proto",
        "inference_server_patched.proto",
    ]

    native.genrule(
        name = name,
        srcs = [base_sources, config] + extension_protos,
        outs = output_files,
        tools = ["@michelangelo//tools/proto-patcher"],
        cmd = """
        # Get output directory
        OUTDIR=$$(dirname $(location {first_out}))

        # Run patch compiler
        $(location @michelangelo//tools/proto-patcher) \\
            --base_protos="$(cat $(location {base}))" \\
            --config=$(location {config}) \\
            --ext_protos="$(locations {ext})" \\
            --field_prefix={prefix} \\
            --tag_start={tag} \\
            --output_dir=$$OUTDIR
        """.format(
            first_out = output_files[0],
            base = base_sources,
            config = config,
            ext = " ".join(["$(location %s)" % p for p in extension_protos]),
            prefix = field_prefix,
            tag = tag_start,
        ),
    )


