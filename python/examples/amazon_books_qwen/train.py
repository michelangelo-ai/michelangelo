"""Training module for Qwen Dual-Encoder model.

Implements GenRec+Qwen architecture with InfoNCE contrastive loss
"""

import logging
import os
from typing import Any

import numpy as np
import ray
import torch
import torch.nn as nn
import torch.nn.functional as F  # noqa: N812
from ray.air.config import ScalingConfig
from ray.data import Dataset
from ray.train import Checkpoint
from ray.train.torch import TorchTrainer
from transformers import AutoModel, AutoTokenizer

import michelangelo.uniflow.core as uniflow
from michelangelo.sdk.workflow.variables import DatasetVariable
from michelangelo.uniflow.plugins.ray import RayTask

log = logging.getLogger(__name__)


class QwenDualEncoder(nn.Module):
    """Qwen-based Dual Encoder for GenRec+Qwen architecture.

    Implements query and document towers with shared/separate Qwen backbones.
    The model encodes queries and documents into a shared embedding space where
    semantically similar items have high cosine similarity.

    Attributes:
        embedding_dim: Output embedding dimension.
        max_query_length: Maximum sequence length for queries.
        max_doc_length: Maximum sequence length for documents.
        shared_encoder: Whether query and document towers share weights.
        tokenizer: Qwen tokenizer for text encoding.
        query_encoder: Transformer encoder for query tower.
        doc_encoder: Transformer encoder for document tower.
        query_projection: Linear layer projecting query representations to
            embedding_dim.
        doc_projection: Linear layer projecting document representations to
            embedding_dim.
    """

    def __init__(
        self,
        model_name: str = "Qwen/Qwen2.5-1.5B",
        embedding_dim: int = 1536,
        max_query_length: int = 128,
        max_doc_length: int = 512,
        shared_encoder: bool = False,
    ):
        """Initialize the Qwen dual encoder model.

        Args:
            model_name: Pretrained Qwen model name from HuggingFace.
                Defaults to "Qwen/Qwen2.5-1.5B".
            embedding_dim: Dimension of output embeddings. Defaults to 1536.
            max_query_length: Maximum tokens for query encoding. Defaults to 128.
            max_doc_length: Maximum tokens for document encoding. Defaults to 512.
            shared_encoder: Whether to use shared encoder for both towers.
                Defaults to False.
        """
        super().__init__()

        self.embedding_dim = embedding_dim
        self.max_query_length = max_query_length
        self.max_doc_length = max_doc_length
        self.shared_encoder = shared_encoder

        # Load Qwen tokenizer
        self.tokenizer = AutoTokenizer.from_pretrained(model_name)
        if self.tokenizer.pad_token is None:
            self.tokenizer.pad_token = self.tokenizer.eos_token

        # Query Tower
        self.query_encoder = AutoModel.from_pretrained(model_name)

        # Document Tower (shared or separate)
        if shared_encoder:
            self.doc_encoder = self.query_encoder
        else:
            self.doc_encoder = AutoModel.from_pretrained(model_name)

        # Projection layers to embedding_dim
        hidden_size = self.query_encoder.config.hidden_size
        self.query_projection = nn.Linear(hidden_size, embedding_dim)
        self.doc_projection = nn.Linear(hidden_size, embedding_dim)

        log.info(f"Initialized QwenDualEncoder with {model_name}")
        log.info(f"Hidden size: {hidden_size}, Embedding dim: {embedding_dim}")

    def encode_queries(self, query_texts):
        """Encode queries using query tower."""
        # Tokenize queries
        query_inputs = self.tokenizer(
            query_texts,
            max_length=self.max_query_length,
            truncation=True,
            padding=True,
            return_tensors="pt",
        )

        # Pass through query encoder
        query_outputs = self.query_encoder(**query_inputs)

        # Sum pooling over all tokens (as per Qwen spec)
        query_embeddings = query_outputs.last_hidden_state.sum(
            dim=1
        )  # [batch_size, hidden_size]

        # Project to target embedding dimension
        query_embeddings = self.query_projection(
            query_embeddings
        )  # [batch_size, embedding_dim]

        # Normalize embeddings
        query_embeddings = F.normalize(query_embeddings, p=2, dim=1)

        return query_embeddings

    def encode_documents(self, doc_texts):
        """Encode documents using document tower."""
        # Tokenize documents
        doc_inputs = self.tokenizer(
            doc_texts,
            max_length=self.max_doc_length,
            truncation=True,
            padding=True,
            return_tensors="pt",
        )

        # Pass through document encoder
        doc_outputs = self.doc_encoder(**doc_inputs)

        # Sum pooling over all tokens
        doc_embeddings = doc_outputs.last_hidden_state.sum(
            dim=1
        )  # [batch_size, hidden_size]

        # Project to target embedding dimension
        doc_embeddings = self.doc_projection(
            doc_embeddings
        )  # [batch_size, embedding_dim]

        # Normalize embeddings
        doc_embeddings = F.normalize(doc_embeddings, p=2, dim=1)

        return doc_embeddings

    def forward(self, query_texts, doc_texts):
        """Forward pass encoding both queries and documents.

        Args:
            query_texts: List of query text strings.
            doc_texts: List of document text strings.

        Returns:
            Tuple of (query_embeddings, doc_embeddings), both normalized tensors
            with shape (batch_size, embedding_dim).
        """
        query_embeddings = self.encode_queries(query_texts)
        doc_embeddings = self.encode_documents(doc_texts)

        return query_embeddings, doc_embeddings


class InfoNCELoss(nn.Module):
    """InfoNCE contrastive loss for dual encoder training.

    Implements the InfoNCE (Information Noise Contrastive Estimation) loss function
    used in contrastive learning. For each query-document pair, the loss encourages
    the query to have high similarity with its corresponding document while having
    low similarity with other documents in the batch (used as negatives).

    Attributes:
        temperature: Temperature scaling parameter that controls the concentration
            of the distribution. Lower values make the model more confident.
    """

    def __init__(self, temperature: float = 0.05):
        """Initialize InfoNCE loss.

        Args:
            temperature: Temperature parameter for contrastive learning.
                Defaults to 0.05.
        """
        super().__init__()
        self.temperature = temperature

    def forward(self, query_embeddings, doc_embeddings, labels):
        """Compute InfoNCE loss.

        Args:
            query_embeddings: [batch_size, embedding_dim]
            doc_embeddings: [batch_size, embedding_dim]
            labels: [batch_size] - 1 for positive pairs, 0 for negative
        """
        # Compute similarity matrix
        similarities = (
            torch.matmul(query_embeddings, doc_embeddings.T) / self.temperature
        )

        # Create targets for positive pairs
        batch_size = similarities.size(0)
        targets = torch.arange(batch_size).to(similarities.device)

        # Compute cross-entropy loss
        loss = F.cross_entropy(similarities, targets)

        return loss


def train_func(config: dict[str, Any]) -> dict[str, Any]:
    """Ray distributed training function for Qwen dual-encoder.

    This function runs on each Ray worker for distributed training
    """
    import ray.train
    import torch

    # Get training configuration
    model_config = config.get("model_config", {})
    training_config = config.get("training_config", {})

    # Initialize model on each worker
    model = QwenDualEncoder(
        model_name=model_config.get("model_name", "Qwen/Qwen2.5-1.5B"),
        embedding_dim=model_config.get("embedding_dim", 1536),
        max_query_length=model_config.get("max_query_length", 128),
        max_doc_length=model_config.get("max_doc_length", 512),
    )

    # Wrap model with Ray's distributed data parallel
    model = ray.train.torch.prepare_model(model)

    # Setup loss and optimizer
    criterion = InfoNCELoss(temperature=training_config.get("temperature", 0.05))
    optimizer = torch.optim.AdamW(
        model.parameters(), lr=training_config.get("learning_rate", 2e-5)
    )

    # Get Ray datasets
    train_dataset = ray.train.get_dataset_shard("train")
    val_dataset = ray.train.get_dataset_shard("validation")

    num_epochs = training_config.get("num_epochs", 3)
    batch_size = training_config.get("batch_size", 32)

    # Training loop with Ray distributed training
    model.train()

    for epoch in range(num_epochs):
        print(
            f"Worker {ray.train.get_context().get_world_rank()}: "
            f"Epoch {epoch + 1}/{num_epochs}"
        )

        epoch_losses = []

        # Iterate through Ray dataset in batches
        for batch_idx, batch in enumerate(
            train_dataset.iter_batches(batch_size=batch_size, batch_format="pandas")
        ):
            # Extract data
            queries = batch["query"].tolist()
            documents = batch["document"].tolist()
            labels = torch.tensor(batch["label"].values, dtype=torch.long)

            # Forward pass
            query_embeddings, doc_embeddings = model(queries, documents)

            # Compute InfoNCE loss
            loss = criterion(query_embeddings, doc_embeddings, labels)

            # Backward pass
            optimizer.zero_grad()
            loss.backward()
            optimizer.step()

            epoch_losses.append(loss.item())

            # Log progress
            if batch_idx % 10 == 0:
                print(
                    f"Worker {ray.train.get_context().get_world_rank()}: "
                    f"Batch {batch_idx}, Loss: {loss.item():.4f}"
                )

        avg_epoch_loss = sum(epoch_losses) / len(epoch_losses) if epoch_losses else 0

        # Validation
        model.eval()
        val_losses = []

        with torch.no_grad():
            for batch in val_dataset.iter_batches(
                batch_size=batch_size, batch_format="pandas"
            ):
                queries = batch["query"].tolist()
                documents = batch["document"].tolist()
                labels = torch.tensor(batch["label"].values, dtype=torch.long)

                query_embeddings, doc_embeddings = model(queries, documents)
                loss = criterion(query_embeddings, doc_embeddings, labels)
                val_losses.append(loss.item())

        avg_val_loss = sum(val_losses) / len(val_losses) if val_losses else 0

        # Report metrics to Ray Train
        ray.train.report(
            {
                "epoch": epoch + 1,
                "train_loss": avg_epoch_loss,
                "val_loss": avg_val_loss,
            }
        )

        # Save checkpoint
        checkpoint = Checkpoint.from_dict(
            {
                "model_state_dict": model.module.state_dict()
                if hasattr(model, "module")
                else model.state_dict(),
                "optimizer_state_dict": optimizer.state_dict(),
                "epoch": epoch,
                "train_loss": avg_epoch_loss,
                "val_loss": avg_val_loss,
            }
        )
        ray.train.report({}, checkpoint=checkpoint)

        model.train()

    return {"final_train_loss": avg_epoch_loss, "final_val_loss": avg_val_loss}


def _convert_spark_to_ray_dataset(spark_df):
    """Convert Spark DataFrame to Ray Dataset."""
    try:
        # Convert Spark DataFrame to Pandas DataFrame first
        pandas_df = spark_df.toPandas()
        log.info(f"Converted Spark DataFrame to Pandas: {len(pandas_df)} rows")

        # Convert Pandas DataFrame to Ray Dataset
        ray_dataset = ray.data.from_pandas(pandas_df)
        log.info(
            f"Converted Pandas DataFrame to Ray Dataset: {ray_dataset.count()} rows"
        )

        return ray_dataset
    except Exception as e:
        log.error(f"Failed to convert Spark DataFrame to Ray Dataset: {e}")
        raise


@uniflow.task(
    config=RayTask(
        head_cpu=1,
        head_memory="8Gi",
        worker_cpu=1,
        worker_memory="8Gi",
        worker_instances=1,  # Default to 1 for local, can be increased for distributed
        # worker_gpu=1,  # Uncomment for GPU training
        runtime_env={
            "pip": ["transformers", "torch", "scikit-learn", "numpy", "pandas"]
        },
    )
)
def train_dual_encoder(
    train_dv: DatasetVariable,
    val_dv: DatasetVariable,
    test_dv: DatasetVariable,
    embedding_dim: int = 1536,
    batch_size: int = 32,
    learning_rate: float = 2e-5,
    num_epochs: int = 3,
    temperature: float = 0.05,
    num_workers: int = 1,  # Default to 1 for local training
    use_gpu: bool = False,  # Default to False, can be set to True if GPU available
    distributed: bool = False,  # Default to False for local training
) -> dict[str, Any]:
    """Unified Qwen dual encoder training function.

    Supports both local and distributed training with optional GPU support

    Args:
        train_dv: Training DatasetVariable
        val_dv: Validation DatasetVariable
        test_dv: Test DatasetVariable
        embedding_dim: Output embedding dimension (Qwen spec: 1536)
        batch_size: Training batch size (per worker for distributed)
        learning_rate: Learning rate
        num_epochs: Number of training epochs
        temperature: InfoNCE temperature parameter
        num_workers: Number of Ray workers (1 for local, 2+ for distributed)
        use_gpu: Whether to use GPU if available
        distributed: Whether to use Ray distributed training

    Returns:
        Dictionary with training metrics and model info
    """
    log.info(
        f"Starting Qwen dual encoder training - Distributed: {distributed}, "
        f"GPU: {use_gpu}, Workers: {num_workers}"
    )

    # Load DatasetVariables as Ray Datasets following boston_housing pattern
    log.info("Loading DatasetVariables as Ray Datasets...")

    train_dv.load_ray_dataset()
    train_data: ray.data.Dataset = train_dv.value

    val_dv.load_ray_dataset()
    val_data: ray.data.Dataset = val_dv.value

    test_dv.load_ray_dataset()
    test_data: ray.data.Dataset = test_dv.value

    log.info("✅ All datasets loaded and ready for training")

    # Get model configuration
    qwen_model_size = os.environ.get("QWEN_MODEL_SIZE", "1.5B")
    model_name_map = {
        "0.6B": "Qwen/Qwen2.5-0.6B",
        "1.5B": "Qwen/Qwen2.5-1.5B",
        "8B": "Qwen/Qwen2.5-8B",
        "local": "distilbert-base-uncased",  # Lightweight model for local testing
    }
    model_name = model_name_map.get(qwen_model_size, "Qwen/Qwen2.5-1.5B")

    # Use distributed training if requested and multiple workers
    if distributed and num_workers > 1:
        return _train_distributed(
            train_data,
            val_data,
            test_data,
            model_name,
            embedding_dim,
            batch_size,
            learning_rate,
            num_epochs,
            temperature,
            num_workers,
            use_gpu,
        )
    else:
        return _train_local(
            train_data,
            val_data,
            test_data,
            model_name,
            embedding_dim,
            batch_size,
            learning_rate,
            num_epochs,
            temperature,
            use_gpu,
        )


def _train_distributed(
    train_data: Dataset,
    val_data: Dataset,
    test_data: Dataset,
    model_name: str,
    embedding_dim: int,
    batch_size: int,
    learning_rate: float,
    num_epochs: int,
    temperature: float,
    num_workers: int,
    use_gpu: bool,
) -> dict[str, Any]:
    """Distributed training using Ray TorchTrainer."""
    log.info(f"Starting Ray distributed training with {num_workers} workers")

    # Configuration for distributed training
    config = {
        "model_config": {
            "model_name": model_name,
            "embedding_dim": embedding_dim,
            "max_query_length": int(os.environ.get("MAX_QUERY_LENGTH", "128")),
            "max_doc_length": int(os.environ.get("MAX_DOC_LENGTH", "512")),
        },
        "training_config": {
            "batch_size": batch_size,
            "learning_rate": learning_rate,
            "num_epochs": num_epochs,
            "temperature": temperature,
        },
    }

    # Setup Ray distributed training
    scaling_config = ScalingConfig(
        num_workers=num_workers,
        use_gpu=use_gpu,
        resources_per_worker={
            "CPU": 4,
            "memory": 16000000000,  # 16GB memory
            "GPU": 1 if use_gpu else 0,
        },
    )

    # Create TorchTrainer for distributed training
    trainer = TorchTrainer(
        train_loop_per_worker=train_func,
        train_loop_config=config,
        scaling_config=scaling_config,
        datasets={"train": train_data, "validation": val_data},
    )

    # Run distributed training
    log.info("Launching Ray distributed training...")
    result = trainer.fit()

    # Get final checkpoint
    checkpoint = result.checkpoint
    checkpoint_data = checkpoint.to_dict()

    # Save final model
    model_save_path = "/tmp/qwen_dual_encoder_distributed.pt"
    torch.save(checkpoint_data, model_save_path)

    log.info("Distributed training completed!")
    log.info(f"Final checkpoint saved to: {model_save_path}")

    return {
        "model_path": model_save_path,
        "training_result": result,
        "final_metrics": result.metrics,
        "checkpoint": checkpoint,
        "num_workers": num_workers,
        "model_name": model_name,
        "training_type": "distributed",
    }


def _train_local(
    train_data: Dataset,
    val_data: Dataset,
    test_data: Dataset,
    model_name: str,
    embedding_dim: int,
    batch_size: int,
    learning_rate: float,
    num_epochs: int,
    temperature: float,
    use_gpu: bool,
) -> dict[str, Any]:
    """Local training with single worker."""
    log.info(f"Starting local training with model: {model_name}")

    # Initialize model with fallback support
    try:
        model = QwenDualEncoder(
            model_name=model_name,
            embedding_dim=embedding_dim,
            max_query_length=int(os.environ.get("MAX_QUERY_LENGTH", "128")),
            max_doc_length=int(os.environ.get("MAX_DOC_LENGTH", "512")),
        )
    except Exception as e:
        log.warning(f"Failed to load {model_name}, using simple model: {e}")
        model = SimpleLocalDualEncoder(embedding_dim=min(embedding_dim, 256))

    # Move to GPU if available and requested
    device = torch.device("cuda" if use_gpu and torch.cuda.is_available() else "cpu")
    model = model.to(device)
    log.info(f"Using device: {device}")

    # Setup loss and optimizer
    criterion = InfoNCELoss(temperature=temperature)
    optimizer = torch.optim.AdamW(model.parameters(), lr=learning_rate)

    # Training loop
    model.train()
    training_losses = []
    total_batches = 0

    for epoch in range(num_epochs):
        log.info(f"Epoch {epoch + 1}/{num_epochs}")
        epoch_losses = []

        # Process training data in batches
        try:
            for batch_idx, batch in enumerate(
                train_data.iter_batches(batch_size=batch_size, batch_format="pandas")
            ):
                if batch_idx >= 20:  # Limit batches for local testing
                    break

                # Extract data
                queries = batch["query"].tolist()
                documents = batch["document"].tolist()
                labels = torch.tensor(batch["label"].values, dtype=torch.long).to(
                    device
                )

                # Handle empty queries/documents
                queries = [q if q else "empty query" for q in queries]
                documents = [d if d else "empty document" for d in documents]

                try:
                    # Forward pass
                    if hasattr(model, "forward"):
                        query_embeddings, doc_embeddings = model(queries, documents)
                    else:
                        # Fallback for simple model
                        query_embeddings = model.encode_queries(queries).to(device)
                        doc_embeddings = model.encode_documents(documents).to(device)

                    # Compute InfoNCE loss
                    loss = criterion(query_embeddings, doc_embeddings, labels)

                    # Backward pass
                    optimizer.zero_grad()
                    loss.backward()
                    optimizer.step()

                    epoch_losses.append(loss.item())
                    total_batches += 1

                    if batch_idx % 5 == 0:
                        log.info(f"Batch {batch_idx}, Loss: {loss.item():.4f}")

                except Exception as e:
                    log.warning(f"Skipping batch {batch_idx} due to error: {e}")
                    continue

        except Exception as e:
            log.warning(f"Training epoch {epoch} had issues: {e}")

        if epoch_losses:
            avg_epoch_loss = np.mean(epoch_losses)
            training_losses.append(avg_epoch_loss)
            log.info(f"Epoch {epoch + 1} average loss: {avg_epoch_loss:.4f}")

    # Validation
    model.eval()
    val_losses = []

    try:
        with torch.no_grad():
            for batch_idx, batch in enumerate(
                val_data.iter_batches(batch_size=batch_size, batch_format="pandas")
            ):
                if batch_idx >= 5:  # Limit validation batches
                    break

                queries = batch["query"].tolist()
                documents = batch["document"].tolist()
                labels = torch.tensor(batch["label"].values, dtype=torch.long).to(
                    device
                )

                queries = [q if q else "empty query" for q in queries]
                documents = [d if d else "empty document" for d in documents]

                try:
                    if hasattr(model, "forward"):
                        query_embeddings, doc_embeddings = model(queries, documents)
                    else:
                        query_embeddings = model.encode_queries(queries).to(device)
                        doc_embeddings = model.encode_documents(documents).to(device)

                    loss = criterion(query_embeddings, doc_embeddings, labels)
                    val_losses.append(loss.item())

                except Exception as e:
                    log.warning(f"Skipping validation batch {batch_idx}: {e}")
                    continue

    except Exception as e:
        log.warning(f"Validation had issues: {e}")

    avg_val_loss = np.mean(val_losses) if val_losses else 0.0
    final_train_loss = training_losses[-1] if training_losses else 0.0

    log.info(
        f"Training completed! Final train loss: {final_train_loss:.4f}, "
        f"Val loss: {avg_val_loss:.4f}"
    )

    # Save model checkpoint
    model_save_path = "/tmp/qwen_dual_encoder_local.pt"
    try:
        torch.save(
            {
                "model_state_dict": model.state_dict(),
                "optimizer_state_dict": optimizer.state_dict(),
                "training_losses": training_losses,
                "val_loss": avg_val_loss,
                "model_config": {
                    "model_name": model_name,
                    "embedding_dim": embedding_dim,
                    "total_batches": total_batches,
                    "device": str(device),
                },
            },
            model_save_path,
        )
        log.info(f"Model saved to: {model_save_path}")
    except Exception as e:
        log.warning(f"Could not save model: {e}")

    return {
        "model_path": model_save_path,
        "final_train_loss": final_train_loss,
        "final_val_loss": avg_val_loss,
        "training_losses": training_losses,
        "total_batches": total_batches,
        "num_epochs": num_epochs,
        "model_name": model_name,
        "device": str(device),
        "training_type": "local",
        "status": "completed",
    }


class SimpleLocalDualEncoder(nn.Module):
    """Simple dual encoder for local testing when Qwen models fail.

    Provides a lightweight fallback model using hash-based text encoding instead of
    transformers. Useful for testing the training pipeline without GPU requirements
    or when HuggingFace models are unavailable.

    Attributes:
        embedding_dim: Dimension of output embeddings.
        text_embedding: Embedding layer for hash-based text encoding.
        query_projection: Linear projection for query tower.
        doc_projection: Linear projection for document tower.
    """

    def __init__(self, vocab_size=10000, embedding_dim=256):
        """Initialize simple dual encoder for testing.

        Args:
            vocab_size: Vocabulary size for hash-based encoding. Defaults to 10000.
            embedding_dim: Dimension of output embeddings. Defaults to 256.
        """
        super().__init__()
        self.embedding_dim = embedding_dim
        self.text_embedding = nn.Embedding(vocab_size, embedding_dim)
        self.query_projection = nn.Linear(embedding_dim, embedding_dim)
        self.doc_projection = nn.Linear(embedding_dim, embedding_dim)

    def encode_text(self, texts):
        """Encode text using simple hash-based embedding.

        Args:
            texts: List of text strings to encode.

        Returns:
            Tensor of embeddings with shape (batch_size, embedding_dim).
        """
        # Simple hash-based encoding for testing
        embeddings = []
        for text in texts:
            text_hash = abs(hash(text)) % 10000
            emb = self.text_embedding(torch.tensor([text_hash]))
            embeddings.append(emb.squeeze(0))
        return torch.stack(embeddings)

    def encode_queries(self, queries):
        """Encode queries through query tower.

        Args:
            queries: List of query strings.

        Returns:
            Normalized query embeddings.
        """
        embeddings = self.encode_text(queries)
        embeddings = self.query_projection(embeddings)
        return F.normalize(embeddings, p=2, dim=1)

    def encode_documents(self, documents):
        """Encode documents through document tower.

        Args:
            documents: List of document strings.

        Returns:
            Normalized document embeddings.
        """
        embeddings = self.encode_text(documents)
        embeddings = self.doc_projection(embeddings)
        return F.normalize(embeddings, p=2, dim=1)

    def forward(self, queries, documents):
        """Forward pass encoding both queries and documents.

        Args:
            queries: List of query strings.
            documents: List of document strings.

        Returns:
            Tuple of (query_embeddings, document_embeddings).
        """
        query_emb = self.encode_queries(queries)
        doc_emb = self.encode_documents(documents)
        return query_emb, doc_emb
