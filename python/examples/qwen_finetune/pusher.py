import os
import logging
import tempfile
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask
import fsspec
import mlflow.pytorch
from transformers import AutoTokenizer
from michelangelo.gen.api.v2.model_pb2 import Model, ModelKind
from michelangelo.api.v2.client import APIClient

log = logging.getLogger(__name__)


@uniflow.task(
    config=RayTask(
        head_cpu=2,
        head_memory="8Gi",
        worker_cpu=2,
        worker_memory="8Gi",
        worker_instances=1,
        breakpoint=False,
    ),
    cache_enabled=True,
)
def pusher(model_uri: str, deployed_model_name: str):
    """
    Push the fine-tuned Qwen model for LLM-D deployment.
    
    For LLM-D, we need to upload the model to the expected S3 structure
    and create a Model resource that LLM-D can use.
    """
    # Load the fine-tuned model
    model = mlflow.pytorch.load_model(model_uri, map_location="cpu")
    
    # For LLM-D deployment, we typically need the model files in HuggingFace format
    # Let's save the model in the correct format for LLM-D
    
    with tempfile.TemporaryDirectory() as temp_dir:
        local_model_dir = os.path.join(temp_dir, deployed_model_name)
        os.makedirs(local_model_dir, exist_ok=True)
        
        # Save model in HuggingFace format
        model.save_pretrained(local_model_dir)
        
        # Also save the tokenizer
        tokenizer = AutoTokenizer.from_pretrained("Qwen/Qwen1.5-1.8B-Chat", trust_remote_code=True)
        tokenizer.save_pretrained(local_model_dir)
        
        # Define deployment bucket and paths for LLM-D
        deploy_bucket = "deploy-models"
        base_s3_path = f"s3://{deploy_bucket}/{deployed_model_name}"
        
        # Upload all model files to S3
        log.info(f"Uploading model files from {local_model_dir} to {base_s3_path}")
        
        # Walk through all files in the model directory
        for root, dirs, files in os.walk(local_model_dir):
            for file in files:
                local_file_path = os.path.join(root, file)
                # Get relative path from model directory
                rel_path = os.path.relpath(local_file_path, local_model_dir)
                s3_uri = f"{base_s3_path}/{rel_path}"
                
                # Upload file
                with fsspec.open(s3_uri, mode="wb") as s3_file:
                    with open(local_file_path, "rb") as local_f:
                        s3_file.write(local_f.read())
                log.info(f"Uploaded {local_file_path} to {s3_uri}")
        
        log.info(f"All model files uploaded to {base_s3_path}")

    # Create or update the Model resource in Michelangelo
    namespace = "default"
    name = deployed_model_name

    # Check if model already exists
    retrieved_model = None
    APIClient.set_caller("uniflow-client")
    try:
        retrieved_model = APIClient.ModelService.get_model(
            namespace=namespace, name=name
        )
        log.info("Retrieved existing model:")
        log.info(retrieved_model)
    except Exception as e:
        log.info(f"Model does not exist yet: {e}")

    if not retrieved_model:
        # Create new model
        model = Model()
        model.metadata.namespace = namespace
        model.metadata.name = name

        # Define model spec for LLM
        model.spec.owner.name = "default-user"
        model.spec.description = "Fine-tuned Qwen model for LLM-D deployment"
        model.spec.kind = ModelKind.MODEL_KIND_GENERATIVE_LLM
        model.spec.algorithm = "qwen"
        model.spec.training_framework = "pytorch"
        model.spec.source = "Michelangelo V2"

        # Set the S3 path as the deployable artifact
        # For LLM-D, this should point to the HuggingFace model directory
        model.spec.deployable_artifact_uri.extend([base_s3_path, model_uri])

        try:
            response = APIClient.ModelService.create_model(model)
            log.info("Created model successfully:")
            log.info(response)
        except Exception as e:
            raise e
    else:
        # Update existing model
        retrieved_model.spec.ClearField("deployable_artifact_uri")
        retrieved_model.spec.deployable_artifact_uri.extend([
            base_s3_path,
            model_uri,
        ])
        
        try:
            response = APIClient.ModelService.update_model(retrieved_model)
            log.info("Updated model:")
            log.info(response)
        except Exception as e:
            log.error(f"Error updating model: {e}")
            raise e

    return name