from logging import getLogger

from grpc import Channel

from mactl import CRD
from plugins.pipeline.apply import generate_apply
from plugins.pipeline.create import generate_create
from plugins.pipeline.run import generate_run
from plugins.pipeline.run import convert_crd_metadata_pipeline_run, generate_run
from mactl import CRD


_LOG = getLogger(__name__)


def apply_plugins(
    crd: CRD, target_command: str, crds: dict[str, CRD], channel: Channel
):
    """
    Apply plugins to the crd.
    """
    _LOG.info("Applying plugins to crd: %r / %r", crd, target_command)
    if target_command == "apply":
        generate_apply(crd, channel)
    if target_command == "create":
        generate_create(crd, channel)
    _LOG.info("Plugins applied successfully to crd: %s", crd)


def handle_pipeline_command(args: list[str], kwargs: dict[str, str], channel: Channel):
    """
    Handle pipeline-specific commands like 'run'.
    
    Args:
        args: Command arguments (e.g., ["pipeline", "run"])
        kwargs: Command options (e.g., {"namespace": "test", "name": "pipeline"})
        channel: gRPC channel
    
    Returns:
        Response message or None if command not handled
    """
    if len(args) < 2:
        return None
        
    resource_type = args[0]  
    action = args[1]         
    
    if resource_type != "pipeline":
        return None
        
    if action == "run":
        _LOG.info("Handling pipeline run command")
        pipeline_crd = CRD(name="pipeline", full_name="michelangelo.api.v2.PipelineService")
        pipeline_crd.func_crd_metadata_converter = convert_crd_metadata_pipeline_run
        
        generate_run(pipeline_crd, channel)
        return pipeline_crd.run(**kwargs)
    
    return None
