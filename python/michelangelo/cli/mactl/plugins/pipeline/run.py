import time
import uuid
from logging import getLogger

from grpc import Channel

from mactl import CRD


_LOG = getLogger(__name__)


def generate_run(crd: CRD, channel: Channel):
    """
    Generate run function for pipeline CRD.
    """
    _LOG.info("Generating `pipeline run` crd for: %s", crd)
    
    crd.func_crd_metadata_converter = convert_crd_metadata_pipeline_run
    crd.generate_create(channel)


def convert_crd_metadata_pipeline_run(
    yaml_dict: dict, crd_class: type, yaml_path
) -> dict:
    """
    Convert CRD metadata for pipeline run command.
    This function generates a CreatePipelineRunRequest object from command line arguments.
    """
    _LOG.info("Converting metadata for pipeline run command")
    
    if not isinstance(yaml_dict, dict):
        _LOG.error("Expected a dictionary, got: %r", type(yaml_dict))
        raise ValueError("Expected a dictionary for pipeline run metadata")
    
    # Validate required arguments
    if "namespace" not in yaml_dict:
        raise ValueError("--namespace is required for pipeline run")
    if "name" not in yaml_dict:
        raise ValueError("--name is required for pipeline run")
    
    namespace = yaml_dict["namespace"]
    pipeline_name = yaml_dict["name"]
    
    timestamp = int(time.time())
    uuid8 = str(uuid.uuid4())[:8]
    run_name = f"run-{timestamp}-{uuid8}"
    
    _LOG.info("Generating pipeline run: %s for pipeline: %s in namespace: %s", 
              run_name, pipeline_name, namespace)
    
    # Create pipeline run object
    pipeline_run = generate_pipeline_run_object(
        run_name=run_name,
        pipeline_name=pipeline_name,
        namespace=namespace
    )
    
    return {"pipeline_run": pipeline_run}


def generate_pipeline_run_object(run_name: str, pipeline_name: str, namespace: str) -> dict:
    """
    Generate PipelineRun object as dictionary.
    
    Args:
        run_name: Generated unique name for the pipeline run
        pipeline_name: Name of the target pipeline to run
        namespace: Kubernetes namespace
        
    Returns:
        dict: Configured pipeline run object as dictionary
    """
    
    pipeline_run_dict = {
        "typeMeta": {
            "kind": "PipelineRun",
            "apiVersion": "michelangelo.api/v2"
        },
        "metadata": {
            "name": run_name,
            "namespace": namespace
        },
        "spec": {
            "pipeline": {
                "name": pipeline_name,
                "namespace": namespace
            },
            "actor": {
                "name": "mactl-user" 
            }
        }
    }
    
    _LOG.info("Generated pipeline run object: %s", run_name)
    return pipeline_run_dict