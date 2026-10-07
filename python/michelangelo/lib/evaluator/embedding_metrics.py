"""Intrinsic embedding-quality metrics as TorchMetrics.

Label-free checks on the geometry of an embedding column -- is the space
collapsed, how many dimensions carry variance, does it cluster. Each metric
reads an ``[N, D]`` prediction tensor (the evaluator stacks a list/array
column into one) and returns a single scalar.

They are tagged ``_metric_type = "aggregation"``, so the evaluator calls
``update(preds)`` without a target. ``columns.target_col`` is still required
by the config schema. The metric never reads it, but the evaluator still loads
it from the dataset (without a ``data_preprocessor`` it is part of the parquet
column projection), so point it at a cheap, narrow column such as a small ID
or label column, not a wide one like another embedding:

    - name: user_emb_cosine_mean
      metric: michelangelo.lib.evaluator.embedding_metrics.EmbeddingCosineSimilarity
      params: {stat: mean}
      columns:
        prediction_col: user_embedding
        target_col: user_id   # required by schema; loaded but not read

Every metric works on a seeded random sample of at most ``max_samples`` rows,
which bounds memory and keeps the pairwise metrics at ``max_samples**2`` work
regardless of dataset size, while staying reproducible across runs.
"""

from typing import Any, Union

import torch
from torchmetrics import Metric

__all__ = [
    "EmbeddingCosineSimilarity",
    "EmbeddingEffectiveRank",
    "EmbeddingNorm",
    "EmbeddingSilhouette",
]

_STATS = {"mean": torch.mean, "std": torch.std}


def _check_stat(stat: str) -> str:
    if stat not in _STATS:
        raise ValueError(f"stat must be one of {sorted(_STATS)}, got {stat!r}")
    return stat


def _rank() -> int:
    """This process's rank, so each one draws its own keys; 0 outside a group."""
    if torch.distributed.is_available() and torch.distributed.is_initialized():
        return torch.distributed.get_rank()
    return 0


def _cat(state: Union[list[torch.Tensor], torch.Tensor]) -> torch.Tensor:
    """A list state as one tensor; a distributed sync has already concatenated it."""
    if isinstance(state, torch.Tensor):
        return state
    return torch.cat(state) if state else torch.empty(0)


class _EmbeddingMetric(Metric):
    """Collects a bounded, seeded sample of embedding rows; subclasses score it.

    The sample is bottom-k: every row draws a random key and the
    ``max_samples`` rows with the smallest keys are kept. That is a uniform
    sample of every row seen, however the rows were split into updates, and
    two kept sets merge by concatenating them and keeping the smallest keys
    again -- which is what lets the states cat-reduce across processes. Rows
    are stored flattened so every state is 1-D, which a process that saw no
    rows can still sync.
    """

    higher_is_better = None
    # forward() then updates the global state directly instead of appending
    # each batch's kept set to it, so the state stays bounded there too.
    full_state_update = True
    _metric_type = "aggregation"

    def __init__(self, max_samples: int = 5_000, seed: int = 0, **kwargs: Any) -> None:
        """Create the metric.

        Args:
            max_samples: Most rows kept for scoring; larger inputs are sampled.
            seed: Seed for the row sample, so results are reproducible.
            **kwargs: Passed through to ``torchmetrics.Metric``.
        """
        super().__init__(**kwargs)
        if max_samples < 2:
            raise ValueError(f"max_samples must be >= 2, got {max_samples}")
        self.max_samples = max_samples
        self.seed = seed
        self.add_state("samples", default=[], dist_reduce_fx="cat")
        self.add_state("keys", default=[], dist_reduce_fx="cat")
        # Rows this process has seen; seeds each update's keys, so a run is
        # reproducible and no two updates draw the same keys.
        self.add_state("rows_seen", default=torch.tensor(0), dist_reduce_fx="sum")

    def update(self, preds: torch.Tensor) -> None:
        """Add a batch of ``[N, D]`` embedding rows to the sample."""
        # An empty scope (e.g. a filter that left no rows) arrives as a 1-D
        # empty tensor, since there were no rows to stack.
        if preds.numel() == 0:
            return
        if preds.dim() != 2:
            raise ValueError(
                f"expected [N, D] embeddings, got shape {tuple(preds.shape)}"
            )
        n, d = preds.shape
        generator = torch.Generator().manual_seed(
            hash((self.seed, _rank(), int(self.rows_seen)))
        )
        keys = torch.rand(n, generator=generator).to(preds.device)
        rows = torch.cat([*self.samples, preds.float().flatten()]).view(-1, d)
        rows, keys = self._smallest_keys(rows, torch.cat([*self.keys, keys]))
        self.samples = [rows.flatten()]
        self.keys = [keys]
        self.rows_seen += n

    def compute(self) -> torch.Tensor:
        """Score the collected sample; nan when no rows were seen."""
        # A distributed sync hands the list states over already concatenated.
        keys = _cat(self.keys)
        if keys.numel() == 0:
            return torch.tensor(float("nan"))
        rows = _cat(self.samples).view(keys.numel(), -1)
        return self._score(self._smallest_keys(rows, keys)[0])

    def _smallest_keys(
        self, rows: torch.Tensor, keys: torch.Tensor
    ) -> tuple[torch.Tensor, torch.Tensor]:
        if keys.numel() <= self.max_samples:
            return rows, keys
        idx = torch.topk(keys, self.max_samples, largest=False).indices
        return rows[idx], keys[idx]

    def _score(self, x: torch.Tensor) -> torch.Tensor:
        raise NotImplementedError


class EmbeddingCosineSimilarity(_EmbeddingMetric):
    """Mean or std of pairwise cosine similarity over distinct row pairs.

    The collapse check: a mean near 1 with a small std means every embedding
    points the same way. A healthy space sits near 0 with a wide spread.
    """

    def __init__(self, stat: str = "mean", **kwargs: Any) -> None:
        """Create the metric.

        Args:
            stat: ``"mean"`` or ``"std"`` of the pairwise similarities.
            **kwargs: Passed through to ``_EmbeddingMetric``.
        """
        super().__init__(**kwargs)
        self.stat = _check_stat(stat)

    def _score(self, x: torch.Tensor) -> torch.Tensor:
        x = torch.nn.functional.normalize(x, dim=1)
        n = x.shape[0]
        upper = torch.triu(
            torch.ones(n, n, dtype=torch.bool, device=x.device), diagonal=1
        )
        return _STATS[self.stat]((x @ x.T)[upper])


class EmbeddingNorm(_EmbeddingMetric):
    """Mean or std of the rows' L2 norms -- flags exploding or vanishing vectors."""

    def __init__(self, stat: str = "mean", **kwargs: Any) -> None:
        """Create the metric.

        Args:
            stat: ``"mean"`` or ``"std"`` of the row norms.
            **kwargs: Passed through to ``_EmbeddingMetric``.
        """
        super().__init__(**kwargs)
        self.stat = _check_stat(stat)

    def _score(self, x: torch.Tensor) -> torch.Tensor:
        return _STATS[self.stat](torch.linalg.vector_norm(x, dim=1))


class EmbeddingEffectiveRank(_EmbeddingMetric):
    """Effective rank of the centered sample (Roy & Vetterli, 2007).

    Computed as ``exp(entropy(s**2 / sum(s**2)))`` over the singular values.
    Ranges from 1 (all variance along one direction) to D (variance spread
    evenly over every dimension); a value far below D means dimensional collapse.
    """

    def _score(self, x: torch.Tensor) -> torch.Tensor:
        s = torch.linalg.svdvals(x - x.mean(dim=0, keepdim=True))
        p = s**2 / (s**2).sum()
        p = p[p > 0]
        return torch.exp(-(p * p.log()).sum())


class EmbeddingSilhouette(_EmbeddingMetric):
    """Silhouette score (-1 to 1) of a K-means clustering of the sample.

    Higher means the space has clearer group structure at ``n_clusters``.
    Returns nan when the sample cannot form at least two clusters.
    """

    def __init__(self, n_clusters: int = 8, **kwargs: Any) -> None:
        """Create the metric.

        Args:
            n_clusters: Number of K-means clusters to score.
            **kwargs: Passed through to ``_EmbeddingMetric``.
        """
        super().__init__(**kwargs)
        if n_clusters < 2:
            raise ValueError(f"n_clusters must be >= 2, got {n_clusters}")
        self.n_clusters = n_clusters

    def _score(self, x: torch.Tensor) -> torch.Tensor:
        # Imported lazily so loading this module does not pay for sklearn.
        from sklearn.cluster import KMeans
        from sklearn.metrics import silhouette_score

        data = x.cpu().numpy()
        if len(data) <= self.n_clusters:
            return torch.tensor(float("nan"))
        labels = KMeans(
            n_clusters=self.n_clusters, random_state=self.seed, n_init=3
        ).fit_predict(data)
        if len(set(labels)) < 2:
            return torch.tensor(float("nan"))
        return torch.tensor(float(silhouette_score(data, labels)))
