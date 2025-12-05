"""Simple GPT Fine-tuning Demo (Local Testing Version)."""

import os

import michelangelo.uniflow.core as uniflow
from examples.gpt_oss_20b_finetune.data import prepare_finetune_dataset
from examples.gpt_oss_20b_finetune.eval import evaluate_gpt_model

# Import simple functions
from examples.gpt_oss_20b_finetune.simple_train import simple_train_gpt
from michelangelo.uniflow.plugins.ray import UF_PLUGIN_RAY_USE_FSSPEC


@uniflow.workflow()
def simple_gpt_workflow(
    dataset_name="alpaca", num_epochs=1, sample_size=100, model_name="gpt2"
):
    """Simple GPT fine-tuning workflow for testing."""
    # Prepare dataset
    train_dv, val_dv, test_dv = prepare_finetune_dataset(
        dataset_name=dataset_name,
        max_length=512,
        sample_size=sample_size,
        model_name=model_name,
    )

    # Train model (returns dict with checkpoint_path)
    train_result = simple_train_gpt(
        train_dv=train_dv,
        validation_dv=val_dv,
        model_name=model_name,
        num_epochs=num_epochs,
        batch_size=1,
        learning_rate=5e-5,
        use_lora=True,
    )

    # SKIP EVALUATION FOR NOW
    # # Evaluate model using checkpoint path
    # evaluate_gpt_model(
    #     test_dv=test_dv,
    #     checkpoint_path=train_result["checkpoint_path"],
    #     model_name=model_name,
    #     use_lora=True,
    #     lora_rank=16,
    #     learning_rate=5e-5,
    #     max_length=512,
    #     batch_size=1,
    #     num_samples=3,  # Very small for testing
    # )

    return True


if __name__ == "__main__":
    print("=" * 60)
    print("Simple GPT Fine-tuning Demo")
    print("=" * 60)

    ctx = uniflow.create_context()

    # Detect deployment environment
    # Set DEPLOY_ENV=gke to use GKE configuration
    deploy_env = os.environ.get("DEPLOY_ENV", "local")
    
    # Common environment setup
    ctx.environ["DATA_SIZE"] = "100"
    ctx.environ[UF_PLUGIN_RAY_USE_FSSPEC] = "0"
    ctx.environ["S3_ALLOW_BUCKET_CREATION"] = "True"
    os.environ["RAY_TRAIN_ENABLE_V2_MIGRATION_WARNINGS"] = "0"

    if ctx.is_local_run():
        # Local development - use localhost with port forwarding
        ctx.environ["MA_NAMESPACE"] = "default"
        ctx.environ["IMAGE_PULL_POLICY"] = "Never"
        ctx.environ["RAY_LOG_URL_PREFIX"] = "http://localhost:9091/logs"
        ctx.environ["SPARK_LOG_URL_PREFIX"] = "http://localhost:9091/logs"
        ctx.environ["MLFLOW_TRACKING_URI"] = "http://localhost:5001"
        ctx.environ["MLFLOW_BACKEND_STORE_URI"] = (
            "mysql+pymysql://root:root@localhost:3306/mlflow"
        )
        ctx.environ["MLFLOW_S3_ENDPOINT_URL"] = "http://localhost:9091"
        ctx.environ["MA_API_SERVER"] = "localhost:14566"
        ctx.environ["MLFLOW_DEFAULT_ARTIFACT_ROOT"] = "s3://mlflow"
        ctx.environ["AWS_ACCESS_KEY_ID"] = "minioadmin"
        ctx.environ["AWS_SECRET_ACCESS_KEY"] = "minioadmin"
    elif deploy_env == "gke":
        # GKE deployment - use GCS and GKE service names
        # Set these env vars before running:
        #   export DEPLOY_ENV=gke
        #   export GCP_PROJECT_ID=michelanglo-oss-196506
        gcp_project_id = os.environ.get("GCP_PROJECT_ID", "michelanglo-oss-196506")
        
        ctx.environ["MA_NAMESPACE"] = "michelangelo"
        ctx.environ["IMAGE_PULL_POLICY"] = "Always"
        ctx.environ["CONTAINER_IMAGE"] = "us-east1-docker.pkg.dev/michelanglo-oss-196506/michelangelo-pipelines/examples:latest"
        ctx.environ["RAY_LOG_URL_PREFIX"] = f"gs://{gcp_project_id}-logs"
        ctx.environ["SPARK_LOG_URL_PREFIX"] = f"gs://{gcp_project_id}-logs"
        ctx.environ["MLFLOW_TRACKING_URI"] = "http://mlflow:5000"
        ctx.environ["MLFLOW_BACKEND_STORE_URI"] = (
            "mysql+pymysql://root:root@mysql:3306/mlflow"
        )
        ctx.environ["MA_API_SERVER"] = "michelangelo-apiserver:14566"
        # GCS storage - uses Workload Identity, no credentials needed
        ctx.environ["MLFLOW_DEFAULT_ARTIFACT_ROOT"] = f"gs://{gcp_project_id}-mlflow"
        # Empty credentials = use GCP Workload Identity / default credentials
        ctx.environ["AWS_ACCESS_KEY_ID"] = ""
        ctx.environ["AWS_SECRET_ACCESS_KEY"] = ""
        # Enable GPU tolerations for GKE clusters with nvidia.com/gpu taints
        ctx.environ["RAY_ENABLE_GPU_TOLERATIONS"] = "true"
    else:
        # Local cluster deployment (k3d/sandbox) - use MinIO
        ctx.environ["MA_NAMESPACE"] = "default"
        ctx.environ["IMAGE_PULL_POLICY"] = "Never"
        ctx.environ["RAY_LOG_URL_PREFIX"] = "http://minio:9091/logs"
        ctx.environ["SPARK_LOG_URL_PREFIX"] = "http://minio:9091/logs"
        ctx.environ["MLFLOW_TRACKING_URI"] = "http://mlflow-proxy:5001"
        ctx.environ["MLFLOW_BACKEND_STORE_URI"] = (
            "mysql+pymysql://root:root@mysql:3306/mlflow"
        )
        ctx.environ["MLFLOW_S3_ENDPOINT_URL"] = "http://minio:9091"
        ctx.environ["MA_API_SERVER"] = "michelangelo-apiserver:14566"
        ctx.environ["MLFLOW_DEFAULT_ARTIFACT_ROOT"] = "s3://mlflow"
        ctx.environ["AWS_ACCESS_KEY_ID"] = "minioadmin"
        ctx.environ["AWS_SECRET_ACCESS_KEY"] = "minioadmin"

    # Run the workflow
    ctx.run(
        simple_gpt_workflow,
        dataset_name="alpaca",
        num_epochs=3,
        sample_size=50,
        model_name="gpt2",
    )
