"""
MLflow model packager for GPT fine-tuned models
Packages and registers models using MLflow model registry with S3 storage
"""

import logging
import os
import tempfile
from typing import Dict, Any, Optional
import mlflow
import mlflow.pytorch
import torch
import numpy as np
from transformers import AutoModelForCausalLM, AutoTokenizer
from peft import PeftModel

# Michelangelo API v2 imports for model entity creation
from michelangelo.api.v2 import APIClient
from michelangelo.gen.api.v2.model_pb2 import Model, ModelSpec, LLMSpec
from michelangelo.gen.api.v2.user_pb2 import UserInfo
from michelangelo.gen.k8s.io.apimachinery.pkg.apis.meta.v1.generated_pb2 import ObjectMeta, TypeMeta

log = logging.getLogger(__name__)


class MLflowModelPackager:
    """
    MLflow-based model packager for GPT fine-tuned models
    Handles model registration, versioning, and S3 storage integration
    """

    def __init__(self, experiment_name: str = "gpt-finetune-experiment",
                 artifact_location: str = "s3://mlflow"):
        self.experiment_name = experiment_name
        self.artifact_location = artifact_location
        self._setup_experiment()

    def _setup_experiment(self):
        """Setup MLflow experiment"""
        try:
            # Check if experiment already exists
            experiment = mlflow.get_experiment_by_name(self.experiment_name)
            if experiment is None:
                mlflow.create_experiment(
                    name=self.experiment_name,
                    artifact_location=self.artifact_location
                )
                log.info(f"✅ Created MLflow experiment: {self.experiment_name}")
            else:
                log.info(f"✅ Using existing MLflow experiment: {self.experiment_name}")

            mlflow.set_experiment(self.experiment_name)
        except Exception as e:
            log.warning(f"Could not setup MLflow experiment: {e}")

    def package_gpt_model(
        self,
        model_path: str,
        model_name: str = "gpt2",
        model_registry_name: str = "GPTFinetunedModel",
        run_name: Optional[str] = None,
        description: Optional[str] = None,
        tags: Optional[Dict[str, str]] = None,
        create_model_cr: bool = False,
        namespace: str = "default"
    ) -> Dict[str, Any]:
        """
        Package and register a fine-tuned GPT model with MLflow

        Args:
            model_path: Path to the trained model (LoRA adapters)
            model_name: Base model name used for training
            model_registry_name: Name to register in MLflow Model Registry
            run_name: Optional name for the MLflow run
            description: Model description
            tags: Optional tags for the model

        Returns:
            Dictionary with packaging information including model URI
        """
        log.info(f"📦 Starting MLflow packaging for model at: {model_path}")

        # Set default values
        if run_name is None:
            run_name = f"gpt-finetune-{model_name.replace('/', '-')}"

        if description is None:
            description = f"Fine-tuned {model_name} model with LoRA adapters"

        if tags is None:
            tags = {
                "model_type": "gpt_finetuned",
                "base_model": model_name,
                "framework": "pytorch",
                "technique": "lora"
            }

        try:
            with mlflow.start_run(run_name=run_name):
                # Log model parameters and metadata
                mlflow.log_param("base_model", model_name)
                mlflow.log_param("model_path", model_path)
                mlflow.log_param("training_technique", "lora")

                # Log tags
                for key, value in tags.items():
                    mlflow.set_tag(key, value)

                # Load and package the model
                model_artifacts = self._prepare_model_artifacts(model_path, model_name)

                # Log model with MLflow
                model_uri = self._log_model_to_mlflow(
                    model_artifacts,
                    model_name,
                    description
                )

                # Register model in MLflow Model Registry
                registered_model = self._register_model(
                    model_uri,
                    model_registry_name,
                    description
                )

                # Log additional metrics from training if available
                self._log_training_metrics(model_path)

                # Create Model Custom Resource in Michelangelo (optional)
                model_cr = None
                if create_model_cr:
                    model_cr = self._create_model_entity(
                        model_registry_name,
                        model_name,
                        model_uri,
                        description,
                        tags,
                        registered_model,
                        namespace
                    )

                run_id = mlflow.active_run().info.run_id

                log.info(f"✅ Model packaged successfully!")
                log.info(f"   Run ID: {run_id}")
                log.info(f"   Model URI: {model_uri}")
                log.info(f"   Registry Name: {model_registry_name}")
                if model_cr:
                    try:
                        model_cr_namespace = model_cr.metadata.namespace if hasattr(model_cr, 'metadata') else namespace
                        model_cr_name = model_cr.metadata.name if hasattr(model_cr, 'metadata') else model_registry_name
                        log.info(f"   Model CR: {model_cr_namespace}/{model_cr_name}")
                    except Exception as e:
                        log.debug(f"Could not access Model CR metadata: {e}")

                return {
                    "status": "success",
                    "model_uri": model_uri,
                    "run_id": run_id,
                    "model_registry_name": model_registry_name,
                    "model_version": registered_model.version if registered_model else None,
                    "artifact_location": self.artifact_location,
                    "experiment_name": self.experiment_name,
                    "model_cr": {
                        "namespace": model_cr.metadata.namespace if model_cr and hasattr(model_cr, 'metadata') else namespace,
                        "name": model_cr.metadata.name if model_cr and hasattr(model_cr, 'metadata') else model_registry_name
                    } if model_cr else None
                }

        except Exception as e:
            log.error(f"❌ Failed to package model: {e}")
            return {
                "status": "failed",
                "error": str(e),
                "model_path": model_path
            }

    def _prepare_model_artifacts(self, model_path: str, model_name: str) -> Dict[str, Any]:
        """Prepare model artifacts for MLflow logging"""
        log.info("🔧 Preparing model artifacts...")

        # Load tokenizer
        tokenizer = AutoTokenizer.from_pretrained(model_name)
        if tokenizer.pad_token is None:
            tokenizer.pad_token = tokenizer.eos_token

        # Load base model
        base_model = AutoModelForCausalLM.from_pretrained(
            model_name,
            torch_dtype=torch.float16 if torch.cuda.is_available() else torch.float32,
            device_map="auto" if torch.cuda.is_available() else None
        )

        # Load LoRA adapters if available
        try:
            model = PeftModel.from_pretrained(base_model, model_path)
            log.info("✅ LoRA adapters loaded")
        except Exception as e:
            log.warning(f"Could not load LoRA adapters: {e}, using base model")
            model = base_model

        model.eval()

        # Create artifacts dictionary
        artifacts = {
            "model": model,
            "tokenizer": tokenizer,
            "model_path": model_path,
            "base_model_name": model_name
        }

        # Log model size information
        total_params = sum(p.numel() for p in model.parameters())
        trainable_params = sum(p.numel() for p in model.parameters() if p.requires_grad)

        mlflow.log_metric("total_parameters", total_params)
        mlflow.log_metric("trainable_parameters", trainable_params)
        mlflow.log_metric("trainable_percentage", (trainable_params / total_params) * 100)

        return artifacts

    def _log_model_to_mlflow(
        self,
        artifacts: Dict[str, Any],
        model_name: str,
        description: str
    ) -> str:
        """Log model and artifacts to MLflow"""
        log.info("📤 Logging model to MLflow...")

        # Create a temporary directory for model artifacts
        with tempfile.TemporaryDirectory() as temp_dir:
            model_dir = os.path.join(temp_dir, "model")
            os.makedirs(model_dir, exist_ok=True)

            # Save model and tokenizer
            artifacts["model"].save_pretrained(model_dir)
            artifacts["tokenizer"].save_pretrained(model_dir)

            # Create model info file
            model_info = {
                "base_model": artifacts["base_model_name"],
                "model_type": "gpt_finetuned",
                "training_technique": "lora",
                "framework": "pytorch"
            }

            import json
            with open(os.path.join(model_dir, "model_info.json"), "w") as f:
                json.dump(model_info, f, indent=2)

            # Log the model with MLflow
            mlflow.pytorch.log_model(
                pytorch_model=artifacts["model"],
                artifact_path="gpt_model",
                extra_files=[model_dir],  # Include tokenizer and other files
                conda_env={
                    'channels': ['defaults'],
                    'dependencies': [
                        'python=3.9',
                        'pip',
                        {
                            'pip': [
                                'torch>=1.9.0',
                                'transformers>=4.20.0',
                                'peft>=0.3.0',
                                'mlflow>=2.0.0'
                            ]
                        }
                    ],
                    'name': 'gpt_model_env'
                }
            )

            # Log additional artifacts
            mlflow.log_artifacts(artifacts["model_path"], "original_model")

            # Construct proper MLflow URI
            run_id = mlflow.active_run().info.run_id
            model_uri = f"runs:/{run_id}/gpt_model"

            return model_uri

    def _register_model(
        self,
        model_uri: str,
        model_name: str,
        description: str
    ):
        """Register model in MLflow Model Registry"""
        log.info(f"📋 Registering model in registry: {model_name}")

        try:
            registered_model = mlflow.register_model(
                model_uri=model_uri,
                name=model_name,
                await_registration_for=300  # Wait up to 5 minutes
            )

            # Update model version description
            if registered_model:
                pass  # Skip version description update for simplicity

            log.info(f"✅ Model registered as version {registered_model.version}")
            return registered_model

        except Exception as e:
            log.warning(f"Could not register model: {e}")
            return None

    def _log_training_metrics(self, model_path: str):
        """Log training metrics if available"""
        try:
            # Look for training metrics in the model directory
            metrics_file = os.path.join(model_path, "trainer_state.json")
            if os.path.exists(metrics_file):
                import json
                with open(metrics_file, "r") as f:
                    trainer_state = json.load(f)

                # Log final training loss if available
                if "log_history" in trainer_state and trainer_state["log_history"]:
                    last_log = trainer_state["log_history"][-1]
                    if "train_loss" in last_log:
                        mlflow.log_metric("final_train_loss", last_log["train_loss"])
                    if "eval_loss" in last_log:
                        mlflow.log_metric("final_eval_loss", last_log["eval_loss"])
                        # Calculate perplexity
                        perplexity = np.exp(last_log["eval_loss"])
                        mlflow.log_metric("final_perplexity", perplexity)

        except Exception as e:
            log.debug(f"Could not log training metrics: {e}")

    def list_registered_models(self) -> Dict[str, Any]:
        """List all registered models in the registry"""
        try:
            from mlflow.tracking import MlflowClient
            client = MlflowClient()

            registered_models = client.list_registered_models()

            models_info = []
            for rm in registered_models:
                latest_version = client.get_latest_versions(rm.name, stages=["None", "Production", "Staging"])
                models_info.append({
                    "name": rm.name,
                    "description": rm.description,
                    "latest_version": latest_version[0].version if latest_version else None,
                    "creation_timestamp": rm.creation_timestamp,
                    "last_updated_timestamp": rm.last_updated_timestamp
                })

            return {
                "status": "success",
                "models": models_info,
                "total_count": len(models_info)
            }

        except Exception as e:
            log.error(f"Failed to list registered models: {e}")
            return {
                "status": "failed",
                "error": str(e)
            }

    def load_model(self, model_name: str, version: Optional[str] = None) -> Any:
        """Load a registered model from MLflow"""
        try:
            if version:
                model_uri = f"models:/{model_name}/{version}"
            else:
                model_uri = f"models:/{model_name}/latest"

            log.info(f"🔽 Loading model: {model_uri}")
            model = mlflow.pytorch.load_model(model_uri)

            log.info("✅ Model loaded successfully")
            return model

        except Exception as e:
            log.error(f"Failed to load model {model_name}: {e}")
            raise

    def _create_model_entity(
        self,
        model_registry_name: str,
        base_model_name: str,
        model_uri: str,
        description: str,
        tags: Dict[str, str],
        registered_model=None,
        namespace: str = "default"
    ) -> Optional[Model]:
        """
        Create a Model Custom Resource in Michelangelo API v2

        Args:
            model_registry_name: Name of the model in the registry
            base_model_name: Base model name (e.g., gpt2)
            model_uri: MLflow model URI
            description: Model description
            tags: Model tags
            registered_model: MLflow registered model object
            namespace: Kubernetes namespace for the model CR

        Returns:
            Created Model CR or None if creation failed
        """
        log.info(f"📋 Creating Model Custom Resource: {namespace}/{model_registry_name}")

        try:
            # Set the API client caller
            APIClient.set_caller("mlflow-model-packager")

            # Create model specification
            model_spec = ModelSpec()
            model_spec.description = description
            model_spec.kind = 1  # MODEL_KIND_CUSTOM
            model_spec.algorithm = "transformer"
            model_spec.training_framework = "pytorch"
            model_spec.source = "fine_tuning"
            model_spec.package_type = 3  # DEPLOYABLE_MODEL_PACKAGE_TYPE_RAW

            # Add model artifact URIs - ensure model_uri is string
            model_uri_str = str(model_uri) if hasattr(model_uri, '__str__') else model_uri
            model_spec.model_artifact_uri.extend([model_uri_str])
            model_spec.deployable_artifact_uri.extend([model_uri_str])

            # Create owner information
            owner = UserInfo()
            owner.name = "mlflow-packager"
            model_spec.owner.CopyFrom(owner)

            # Create metadata
            metadata = ObjectMeta()
            metadata.name = model_registry_name.lower().replace("_", "-")  # K8s naming convention
            metadata.namespace = namespace

            # Add labels from tags - fix protobuf type issues
            if hasattr(metadata, 'labels'):
                for key, value in tags.items():
                    metadata.labels[f"michelangelo.ai/{key}"] = str(value)

            # Add annotations - fix protobuf type issues
            if hasattr(metadata, 'annotations'):
                metadata.annotations["michelangelo.ai/mlflow-model-uri"] = str(model_uri)
                metadata.annotations["michelangelo.ai/base-model"] = str(base_model_name)
                if registered_model and hasattr(registered_model, 'version'):
                    metadata.annotations["michelangelo.ai/mlflow-version"] = str(registered_model.version)

            # Create type metadata
            type_meta = TypeMeta()
            type_meta.kind = "Model"
            type_meta.apiVersion = "michelangelo.ai/v2"

            # Create the Model CR
            model_cr = Model()
            model_cr.type_meta.CopyFrom(type_meta)
            model_cr.metadata.CopyFrom(metadata)
            model_cr.spec.CopyFrom(model_spec)

            # Create the model using the API client
            log.info(f"🔄 Attempting API call with model_cr: type={type(model_cr)}")
            created_model = APIClient.ModelService.create_model(model_cr)

            log.info(f"✅ Model CR created successfully: {created_model.metadata.namespace}/{created_model.metadata.name}")
            return created_model

        except Exception as e:
            log.error(f"❌ Failed to create Model CR: {e}")
            log.debug(f"Error details: {str(e)}")
            return None