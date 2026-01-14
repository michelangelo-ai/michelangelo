"""PipelineRun entity plugin module."""

from logging import getLogger
from types import MethodType

from grpc import Channel

from michelangelo.cli.mactl.crd import CRD
from michelangelo.cli.mactl.plugins.entity.pipeline.kill import (
    add_function_signature as add_kill_function_signature,
)
from michelangelo.cli.mactl.plugins.entity.pipeline.kill import generate_kill

_LOG = getLogger(__name__)


def apply_plugins(crd: CRD, channel: Channel):
    """Apply plugin entity function signatures to the CRD.

    This adds the kill function signature for pipeline_run entity.
    """
    _LOG.info("Applying pipeline_run plugin entity to crd: %r", crd)
    _LOG.debug("gRPC Channel: %r", channel)

    add_kill_function_signature(crd)
    crd.generate_kill = MethodType(
        lambda self, ch, parser: generate_kill(self, ch, parser), crd
    )

    _LOG.info("Plugin entities applied successfully to pipeline_run crd: %s", crd)
