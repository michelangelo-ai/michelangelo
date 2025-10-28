from logging import getLogger
from inspect import Signature, Parameter
from types import MethodType

from grpc import Channel
from google.protobuf.message import Message
from google.protobuf.json_format import MessageToDict, ParseDict

from mactl import CRD, bind_signature, METADATA_STUB, get_single_arg


_LOG = getLogger(__name__)


def generate_kill(crd: CRD, channel: Channel):
    """
    Generate kill function for pipeline_run CRD.
    """
    _LOG.info("Generating `pipeline_run kill` for: %s", crd)

    crd.generate_get(channel)

    method_name, input_class, output_class = crd._extract_method_info(
        channel, crd.full_name, "Update"
    )

    kill_func_signature = Signature(
        [Parameter("self", Parameter.POSITIONAL_OR_KEYWORD)]
        + [
            Parameter(name, Parameter.POSITIONAL_OR_KEYWORD)
            for name in ["namespace", "name"]
        ]
        + [Parameter("yes", Parameter.POSITIONAL_OR_KEYWORD, default=[])]
    )

    @bind_signature(kill_func_signature)
    def kill_func(bound_args: Signature) -> Message:
        _LOG.info("Start kill_func for pipeline_run")
        _LOG.info("Bound arguments: %r", bound_args.arguments)
        _self: CRD = bound_args.arguments["self"]
        _name = get_single_arg(bound_args.arguments, "name")
        _namespace = get_single_arg(bound_args.arguments, "namespace")
        _yes = bound_args.arguments.get("yes", [])

        if not _yes:
            resource_type = _self.name.replace("_", " ")
            confirmation = input(f" > kill '{_name}' {resource_type}? [y/N] ")
            if confirmation.lower() not in ["y", "yes"]:
                print("Kill operation cancelled.")
                return None

        current_resource = _self.get(_namespace, _name)
        _LOG.info("Retrieved resource for kill: %r", current_resource)

        current_dict = MessageToDict(current_resource, preserving_proto_field_name=True)

        resource_name = _self.name
        if resource_name in current_dict and "spec" in current_dict[resource_name]:
            current_dict[resource_name]["spec"]["kill"] = True
        else:
            _LOG.error("Missing required spec field in the resource structure")
            raise ValueError(f"Cannot set kill flag on {resource_name}")

        request_input = input_class()
        ParseDict(current_dict, request_input, ignore_unknown_fields=True)

        _LOG.info(
            "KILL Request input (%r) ready: %r",
            type(request_input),
            request_input,
        )

        method_fullname = f"/{_self.full_name}/{method_name}"
        _LOG.info("Method fullname for gRPC call: %s", method_fullname)

        stub_method = channel.unary_unary(
            method_fullname,
            request_serializer=input_class.SerializeToString,
            response_deserializer=output_class.FromString,
        )

        response = stub_method(
            request_input,
            metadata=METADATA_STUB,
            timeout=30,
        )

        response_dict = MessageToDict(response, preserving_proto_field_name=True)
        resource_name = _self.name
        if (
            resource_name in response_dict
            and "spec" in response_dict[resource_name]
            and response_dict[resource_name]["spec"].get("kill") is True
        ):
            _LOG.info("Kill operation successfully set spec.kill=true")
        else:
            _LOG.error("Kill operation failed: spec.kill not set to true in response")
            raise RuntimeError(
                f"Kill operation failed for {resource_name}: spec.kill not properly set"
            )

        _LOG.info("Kill operation completed (%r): %r", type(response), response)
        return response

    kill_func.__signature__ = kill_func_signature
    crd.kill = MethodType(kill_func, crd)
