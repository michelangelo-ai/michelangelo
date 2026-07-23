"""Pipeline `apply` function plugin module."""

from logging import getLogger

from google.protobuf.message import Message
from grpc import RpcError, StatusCode

from michelangelo.cli.mactl.crd import (
    CRD,
    CrdMethodInfo,
    apply_dry_run_to_request,
    crd_method_call,
    get_crd_namespace_and_name_from_yaml,
    read_yaml_to_crd_request,
)

_LOG = getLogger(__name__)


def pipeline_apply_func_impl(
    update_method_info: CrdMethodInfo,
    bound_args,
) -> Message:
    """Pipeline apply implementation.

    update_method_info is passed by generate_apply via the standard partial mechanism.
    """
    _self: CRD = bound_args.arguments["self"]
    _file = bound_args.arguments["file"]
    _dry_run = bound_args.arguments.get("dry_run", False)

    _namespace, _name = get_crd_namespace_and_name_from_yaml(_file)

    message_instance = None
    try:
        message_instance = _self.get(_namespace, _name)
    except RpcError as err:
        _LOG.debug("Pipeline %r / %r does not exist: %r", _namespace, _name, err)
        if err.code() != StatusCode.NOT_FOUND:
            raise

    if message_instance is None:
        # Forward dry_run explicitly — _self.create's bound_args does not
        # inherit apply's dry_run otherwise.
        _LOG.info("Create a new pipeline")
        _self.generate_create(update_method_info.channel)
        return _self.create(_file, dry_run=_dry_run)

    _LOG.info("Updating existing pipeline: %r", message_instance)
    request_input = read_yaml_to_crd_request(
        update_method_info.input_class,
        _self.name,
        _file,
        _self.func_crd_metadata_converter,
    )
    existing = getattr(message_instance, _self.name)
    inner = getattr(request_input, _self.name)
    inner.metadata.resourceVersion = existing.metadata.resourceVersion
    apply_dry_run_to_request(request_input, "update_options", bound_args.arguments)
    call_res = crd_method_call(update_method_info, request_input)
    print(call_res)
    return call_res
