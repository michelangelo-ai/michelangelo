from logging import getLogger
from types import MethodType

from grpc import Channel

from michelangelo.cli.mactl.mactl import CRD
from michelangelo.cli.mactl.plugins.trigger_run.kill import generate_kill


_LOG = getLogger(__name__)


def apply_plugins(
    crd: CRD, target_command: str, crds: dict[str, CRD], channel: Channel
):
    """
    Apply plugins to the crd.
    """
    _LOG.info("Applying plugins to crd: %r / %r", crd, target_command)
    if target_command == "kill":
        crd.generate_kill = MethodType(lambda self, ch: generate_kill(self, ch), crd)
    _LOG.info("Plugins applied successfully to crd: %s", crd)
