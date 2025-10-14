from logging import getLogger

from grpc import Channel

from michelangelo.cli.mactl.mactl import CRD


_LOG = getLogger(__name__)


def apply_plugins(
    crd: CRD, target_command: str, crds: dict[str, CRD], channel: Channel
):
    """
    Apply plugins to the crd.
    """
    _LOG.info("Applying plugins to crd: %r / %r", crd, target_command)
    if target_command == "apply":
        # Use the generic apply functionality for kubectl-independent operation
        _LOG.info("Using generic apply functionality for trigger_run apply")
    else:
        _LOG.warning("Unsupported command for trigger_run: %s. Only 'apply' is supported.", target_command)
    _LOG.info("Plugins applied successfully to crd: %s", crd)