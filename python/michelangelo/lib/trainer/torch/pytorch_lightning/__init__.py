"""PyTorch Lightning trainer wrapping Ray Train.

This package is a one-time snapshot of an internal trainer. Bugs may be patched
in OSS, but new features will not be automatically backported from the source.

Public surface re-exported below:

* :class:`LightningTrainer` — Ray ``TorchTrainer`` subclass that runs a
  PyTorch Lightning training loop.
* :class:`LightningTrainerWithStateDict` — variant that exposes
  :meth:`update_model_state_dict` for loading trained weights into a fresh
  ``torch.nn.Module``.
* :class:`LightningTrainerParam` — dataclass holding the training
  configuration (model factory, datasets, batch size, optional Comet config,
  warm-start specs, etc.).
* :class:`CometParam` — Comet ML logger configuration. Forwarded to
  ``comet_ml`` (imported lazily, so this dataclass is safe to construct
  without ``comet_ml`` installed).
* :class:`TransferLearningSpec`, :class:`IncrementalTrainingSpec`,
  :class:`ModelSpec`, :class:`TrainingType`, :class:`LearningMode` — warm-start
  schema types consumed by the trainer.
"""

from michelangelo.lib.trainer.torch.pytorch_lightning.lightning_trainer import (
    CometParam,
    LightningTrainer,
    LightningTrainerParam,
    LightningTrainerWithStateDict,
)
from michelangelo.lib.trainer.torch.pytorch_lightning.schema import (
    IncrementalTrainingSpec,
    LearningMode,
    ModelSpec,
    TrainingObserver,
    TrainingType,
    TransferLearningSpec,
)

__all__ = [
    "CometParam",
    "IncrementalTrainingSpec",
    "LearningMode",
    "LightningTrainer",
    "LightningTrainerParam",
    "LightningTrainerWithStateDict",
    "ModelSpec",
    "TrainingObserver",
    "TrainingType",
    "TransferLearningSpec",
]
