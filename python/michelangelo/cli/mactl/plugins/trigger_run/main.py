from logging import getLogger

from grpc import Channel

from mactl import CRD
from plugins.trigger_run.kill import generate_kill


_LOG = getLogger(__name__)


def apply_plugins(
    crd: CRD, target_command: str, crds: dict[str, CRD], channel: Channel
):
    """
    Apply plugins to the crd.
    """
    _LOG.info("Applying plugins to crd: %r / %r", crd, target_command)
    if target_command == "kill":
        generate_kill(crd, channel)
    _LOG.info("Plugins applied successfully to crd: %s", crd)
