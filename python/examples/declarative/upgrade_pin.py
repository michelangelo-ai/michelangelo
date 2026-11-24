"""Move a declarative pipeline to another interpreter release.

mactl does not send interpreter_pin yet, so this script calls the Pipeline
API directly. It reads the stored pipeline, sets interpreter_pin.version, and
sends an update. The apiserver writes the artifact, the digest, and
uniflow_tar. It uses the same address, TLS, and metadata settings as mactl.

Usage, from the python/ directory:

    PYTHONPATH=. poetry run python examples/declarative/upgrade_pin.py \
        declarative-test declarative-hello mock-v2 [--dry-run]
"""

import argparse

import grpc

from michelangelo.cli.mactl import config as mactl_config
from michelangelo.gen.api.v2 import (
    pipeline_pb2,
    pipeline_svc_pb2,
    pipeline_svc_pb2_grpc,
)


def main() -> None:
    """Parse the arguments, update the pin, and print the result."""
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("namespace")
    parser.add_argument("name")
    parser.add_argument("version", help="target release version, e.g. mock-v2")
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()

    conf = mactl_config.load_config()
    metadata = [*mactl_config.request_metadata(conf), ("ttl", "600")]
    options = mactl_config.get_channel_options()
    if conf.get("use_tls"):
        channel = grpc.secure_channel(
            conf["address"], grpc.ssl_channel_credentials(), options=options
        )
    else:
        channel = grpc.insecure_channel(conf["address"], options=options)

    with channel:
        stub = pipeline_svc_pb2_grpc.PipelineServiceStub(channel)
        try:
            before, updated = upgrade(stub, metadata, args)
        except grpc.RpcError as err:
            raise SystemExit(f"Error: {err.code().name}: {err.details()}") from None

    manifest = updated.spec.manifest
    print(f"pin: {before} -> {manifest.interpreter_pin.version}")
    print(f"artifact: {manifest.interpreter_pin.artifact}")
    print(f"digest: {manifest.interpreter_pin.digest}")
    print(f"uniflow_tar: {manifest.uniflow_tar}")


def upgrade(stub, metadata, args) -> tuple[str, pipeline_pb2.Pipeline]:
    """Return the old pin version and the pipeline the apiserver wrote."""
    pipeline = stub.GetPipeline(
        pipeline_svc_pb2.GetPipelineRequest(namespace=args.namespace, name=args.name),
        metadata=metadata,
        timeout=30,
    ).pipeline
    before = pipeline.spec.manifest.interpreter_pin.version
    pipeline.spec.manifest.interpreter_pin.CopyFrom(
        pipeline_pb2.InterpreterPin(version=args.version)
    )
    request = pipeline_svc_pb2.UpdatePipelineRequest(pipeline=pipeline)
    if args.dry_run:
        request.update_options.dryRun.append("All")
    updated = stub.UpdatePipeline(request, metadata=metadata, timeout=30).pipeline
    return before, updated


if __name__ == "__main__":
    main()
