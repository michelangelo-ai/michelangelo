"""Ray Tune helper for hyperparameter search on a provisioned Ray cluster.

Public surface re-exported below:

* :func:`tune` -- runs a ``ray.tune.Tuner`` search on the Ray runtime the
  calling process is already connected to and returns a small result dict.
* :class:`TuneParam` -- dataclass holding the search configuration (trainable,
  search space, metric, scheduler, trial budget, ...).
"""

from michelangelo.lib.tuner.ray_tune.tuner import (
    TuneParam,
    tune,
)

__all__ = [
    "TuneParam",
    "tune",
]
