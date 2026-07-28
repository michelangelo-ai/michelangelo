"""Revision entity plugin module."""

from logging import getLogger

from grpc import Channel

from michelangelo.cli.mactl.crd import CRD
from michelangelo.cli.mactl.plugins.entity.revision.get import add_get_filters

_LOG = getLogger(__name__)


def apply_plugins(crd: CRD, channel: Channel, *_, **__):
    """Register the get command filters and columns on the revision CRD."""
    _LOG.info("Applying revision plugin entity to crd: %r", crd)
    _LOG.debug("gRPC Channel: %r", channel)
    add_get_filters(crd)
    _LOG.info("Plugin entities applied successfully to revision crd: %s", crd)
