"""Trigger Run Plugin Application Module."""

# ruff: noqa: I001 -- false positive
from logging import getLogger
from types import MethodType

from grpc import Channel

from michelangelo.cli.mactl.crd import CRD
from michelangelo.cli.mactl.plugins.entity.trigger_run.kill import (
    add_function_signature,
    generate_kill,
)


_LOG = getLogger(__name__)


def apply_plugins(crd: CRD, channel: Channel):
    """Apply plugin entity function signatures to the CRD.

    It adds the necessary function signatures and methods for user commands
    """
    _LOG.info("Applying plugin entity to crd: %r", crd)
    _LOG.debug("gRPC Channel: %r", channel)
    add_function_signature(crd)
    crd.generate_kill = MethodType(
        lambda self, ch, parser: generate_kill(self, ch, parser), crd
    )
    _LOG.info("Plugin entities applied successfully to crd: %s", crd)
