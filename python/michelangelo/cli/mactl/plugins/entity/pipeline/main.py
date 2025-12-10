from logging import getLogger
from types import MethodType

from grpc import Channel

from michelangelo.cli.mactl.crd import CRD
from michelangelo.cli.mactl.plugins.entity.pipeline.create import (
    convert_crd_metadata_pipeline_create,
)
from michelangelo.cli.mactl.plugins.entity.pipeline.dev_run import (
    add_function_signature as add_dev_run_function_signature,
)
from michelangelo.cli.mactl.plugins.entity.pipeline.dev_run import (
    convert_crd_metadata_pipeline_dev_run,
    generate_dev_run,
)
from michelangelo.cli.mactl.plugins.entity.pipeline.run import (
    add_function_signature as add_run_function_signature,
)
from michelangelo.cli.mactl.plugins.entity.pipeline.run import (
    convert_crd_metadata_pipeline_run,
    generate_run,
)

_LOG = getLogger(__name__)


def apply_plugins(crd: CRD, channel: Channel):
    """Apply plugin entity function signatures to the CRD.
    It adds the necessary function signatures and methods for user commands
    """
    _LOG.info("Applying plugin entity to crd: %r", crd)
    _LOG.debug("gRPC Channel: %r", channel)
    add_run_function_signature(crd)
    crd.generate_run = MethodType(
        lambda self, ch, parser: generate_run(self, ch, parser), crd
    )
    add_dev_run_function_signature(crd)
    crd.generate_dev_run = MethodType(
        lambda self, ch, parser: generate_dev_run(self, ch, parser), crd
    )
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
    if target_command == "run":
        crd.func_crd_metadata_converter = convert_crd_metadata_pipeline_run
    if target_command == "dev_run":
        crd.func_crd_metadata_converter = convert_crd_metadata_pipeline_dev_run
    _LOG.info("Plugins applied successfully to crd: %s", crd)
