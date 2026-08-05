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
  configuration (model factory, datasets, batch size, optional Lightning
  logger, warm-start specs, etc.).
* :class:`TransferLearningSpec`, :class:`IncrementalTrainingSpec`,
  :class:`ModelSpec`, :class:`TrainingType`, :class:`LearningMode` — warm-start
  schema types consumed by the trainer.
* :class:`ExperimentStore` — pluggable auto-resume seam;
  :class:`FsspecExperimentStore` is the filesystem default.
* :func:`comet_profiler_sink` — ready-made ``LightningTrainerParam.profiler_sink``
  that ships profiler output to a Comet experiment.
"""

from michelangelo.lib.trainer.torch.pytorch_lightning._private.util import (
    comet_profiler_sink,
)
from michelangelo.lib.trainer.torch.pytorch_lightning.experiment_store import (
    FsspecExperimentStore,
)
from michelangelo.lib.trainer.torch.pytorch_lightning.lightning_trainer import (
    LightningTrainer,
    LightningTrainerParam,
    LightningTrainerWithStateDict,
)
from michelangelo.lib.trainer.torch.pytorch_lightning.schema import (
    ExperimentStore,
    IncrementalTrainingSpec,
    LearningMode,
    ModelSpec,
    TrainingObserver,
    TrainingType,
    TransferLearningSpec,
)

__all__ = [
    "ExperimentStore",
    "FsspecExperimentStore",
    "IncrementalTrainingSpec",
    "LearningMode",
    "LightningTrainer",
    "LightningTrainerParam",
    "LightningTrainerWithStateDict",
    "ModelSpec",
    "TrainingObserver",
    "TrainingType",
    "TransferLearningSpec",
    "comet_profiler_sink",
]
