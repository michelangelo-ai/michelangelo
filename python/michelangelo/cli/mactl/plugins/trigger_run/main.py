from logging import getLogger

from grpc import Channel

from michelangelo.cli.mactl.mactl import CRD
from michelangelo.cli.mactl.plugins.trigger_run.run import generate_run


_LOG = getLogger(__name__)


def apply_plugins(
    crd: CRD, target_command: str, crds: dict[str, CRD], channel: Channel
):
    """
    Apply plugins to the crd.
    """
    _LOG.info("Applying plugins to crd: %r / %r", crd, target_command)
    if target_command == "run":
        generate_run(crd, channel)
    _LOG.info("Plugins applied successfully to crd: %s", crd)