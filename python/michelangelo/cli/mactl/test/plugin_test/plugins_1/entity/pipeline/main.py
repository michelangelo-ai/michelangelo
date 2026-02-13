"""Pipeline entity plugin module."""
from grpc import Channel
from logging import getLogger

from michelangelo.cli.mactl.crd import CRD
from michelangelo.cli.mactl.plugins.entity.pipeline.create import (
    convert_crd_metadata_pipeline_create,
)

_LOG = getLogger(__name__)


def apply_plugins(crd: CRD, channel: Channel):
    """Apply plugin entity function signatures to the CRD.

    It adds the necessary function signatures and methods for user commands
    """
    _LOG.info("Applying plugin entity to crd: %r", crd)
    _LOG.debug("gRPC Channel: %r", channel)
    _LOG.info("Plugin entities applied successfully to crd: %s", crd)


def apply_plugin_command(
    crd: CRD, target_command: str, crds: dict[str, CRD], channel: Channel
):
    """Apply specific target command plugins to the crd."""
    _LOG.info("Applying plugins to crd: %r / %r", crd, target_command)
    _LOG.debug("Available CRDs: %r", crds)
    _LOG.debug("gRPC Channel: %r", channel)
    if target_command == "create":
        crd.func_crd_metadata_converter = convert_crd_metadata_pipeline_create
    _LOG.info("Plugins applied successfully to crd: %s", crd)
