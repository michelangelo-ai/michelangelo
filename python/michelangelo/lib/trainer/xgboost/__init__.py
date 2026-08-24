"""XGBoost trainer wrapping Ray Train.

Public surface re-exported below:

* :class:`XGBoostTrainer` — Ray ``XGBoostTrainer`` subclass that runs a
  data-parallel XGBoost training loop and reports checkpoints through Ray
  Train.
* :class:`XGBoostTrainerParam` — dataclass holding the training configuration
  (label column, datasets, booster params, boosting rounds, ...).
"""

from michelangelo.lib.trainer.xgboost.xgboost_trainer import (
    XGBoostTrainer,
    XGBoostTrainerParam,
)

__all__ = [
    "XGBoostTrainer",
    "XGBoostTrainerParam",
]
