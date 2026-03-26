"""Pipeline entity plugin module."""

from logging import getLogger

from grpc import Channel

from michelangelo.cli.mactl.crd import CRD
from michelangelo.cli.mactl.test.plugin_test.plugins_1.entity.pipeline.apply import (
    convert_crd_metadata_pipeline_apply,
)
from michelangelo.cli.mactl.test.plugin_test.plugins_1.entity.pipeline.create import (
    convert_crd_metadata_pipeline_create,
)
from michelangelo.cli.mactl.test.plugin_test.plugins_1.entity.pipeline.fly import (
    add_fly_function_signature,
)

_LOG = getLogger(__name__)


def apply_plugins(crd: CRD, channel: Channel):
    """Apply plugin entity function signatures to the CRD.

    It adds the necessary function signatures and methods for user commands
    """
    _LOG.info("Applying plugin entity to crd: %r", crd)
    _LOG.debug("gRPC Channel: %r", channel)
    _LOG.info("Plugin entities applied successfully to crd: %s", crd)
    add_fly_function_signature(crd)


def apply_plugin_command(
    crd: CRD, target_command: str, crds: dict[str, CRD], channel: Channel
):
    """Apply specific target command plugins to the crd."""
    _LOG.info("Applying plugins to crd: %r / %r", crd, target_command)
    _LOG.debug("Available CRDs: %r", crds)
    _LOG.debug("gRPC Channel: %r", channel)
    if target_command == "apply":
        crd.func_crd_metadata_converter = convert_crd_metadata_pipeline_apply
    if target_command == "create":
        crd.func_crd_metadata_converter = convert_crd_metadata_pipeline_create
    _LOG.info("Plugins applied successfully to crd: %s", crd)
