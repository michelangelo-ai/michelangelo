"""Pipeline `apply` function plugin module."""

from logging import getLogger

from google.protobuf.message import Message
from grpc import RpcError, StatusCode

from michelangelo.cli.mactl.crd import (
    CRD,
    CrdMethodInfo,
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

    _namespace, _name = get_crd_namespace_and_name_from_yaml(_file)

    message_instance = None
    try:
        message_instance = _self.get(_namespace, _name)
    except RpcError as err:
        _LOG.debug("Pipeline %r / %r does not exist: %r", _namespace, _name, err)
        if err.code() != StatusCode.NOT_FOUND:
            raise

    if message_instance is None:
        _LOG.info("Create a new pipeline")
        _self.generate_create(update_method_info.channel)
        return _self.create(_file)

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
    call_res = crd_method_call(update_method_info, request_input)
    print(call_res)
    return call_res
