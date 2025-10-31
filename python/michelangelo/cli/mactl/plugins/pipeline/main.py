from logging import getLogger
from types import MethodType

from grpc import Channel

from mactl import CRD
from plugins.pipeline.apply import convert_crd_metadata_pipeline_apply
from plugins.pipeline.create import convert_crd_metadata_pipeline_create
from plugins.pipeline.run import generate_run, convert_crd_metadata_pipeline_run
from plugins.pipeline.dev_run import (
    generate_dev_run,
    convert_crd_metadata_pipeline_dev_run,
)


_LOG = getLogger(__name__)


def apply_plugins(
    crd: CRD, target_command: str, crds: dict[str, CRD], channel: Channel
):
    """
    Apply plugins to the crd.
    """
    _LOG.info("Applying plugins to crd: %r / %r", crd, target_command)
    _LOG.debug("Available CRDs: %r", crds)
    _LOG.debug("gRPC Channel: %r", channel)
    if target_command == "apply":
        crd.func_crd_metadata_converter = convert_crd_metadata_pipeline_apply
    if target_command == "create":
        crd.func_crd_metadata_converter = convert_crd_metadata_pipeline_create
    if target_command == "run":
        crd.func_crd_metadata_converter = convert_crd_metadata_pipeline_run
        crd.generate_run = MethodType(lambda self, ch: generate_run(self, ch), crd)
    if target_command == "dev_run":
        crd.func_crd_metadata_converter = convert_crd_metadata_pipeline_dev_run
        crd.generate_dev_run = MethodType(
            lambda self, ch: generate_dev_run(self, ch), crd
        )
    _LOG.info("Plugins applied successfully to crd: %s", crd)
