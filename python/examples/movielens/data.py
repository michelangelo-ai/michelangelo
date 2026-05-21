"""MovieLens-100k data loading for the NCF example.

Downloads the canonical 100k dataset from grouplens.org, builds a contiguous
``user_idx`` / ``item_idx`` mapping, splits 80/20 train/val, and exposes the
splits as :class:`ray.data.Dataset` instances ready for
:class:`michelangelo.lib.trainer.torch.pytorch_lightning.lightning_trainer.LightningTrainer`.
"""

from __future__ import annotations

import io
import logging
import os
import urllib.error
import urllib.request
import zipfile
from dataclasses import dataclass

import numpy as np
import pandas as pd
import ray

# Canonical source. Some sandboxed environments can't reach files.grouplens.org;
# fall back to a github mirror of the same u.data when the canonical URL fails.
_DATA_URL = "https://files.grouplens.org/datasets/movielens/ml-100k.zip"
_UDATA_MIRROR_URL = (
    "https://raw.githubusercontent.com/vinjn/MLStudy/master/data/movielens-100k/u.data"
)
_DEFAULT_CACHE_DIR = "/tmp/movielens_data"
_NETWORK_TIMEOUT_SECONDS = 30

_logger = logging.getLogger(__name__)


@dataclass
class MovieLensSplits:
    """Train/val splits of MovieLens-100k as Ray datasets, plus catalog sizes."""

    train: ray.data.Dataset
    val: ray.data.Dataset
    num_users: int
    num_items: int


def _download_and_extract(cache_dir: str) -> str:
    """Download ml-100k.zip if missing and return the extracted ``u.data`` path.

    Tries the canonical GroupLens URL first; on network error (e.g. restricted
    egress in a sandbox) falls back to a github mirror of the same ``u.data``.
    """
    os.makedirs(cache_dir, exist_ok=True)
    extracted_dir = os.path.join(cache_dir, "ml-100k")
    udata_path = os.path.join(extracted_dir, "u.data")
    if os.path.exists(udata_path):
        _logger.info("MovieLens cache hit at %s", udata_path)
        return udata_path

    try:
        _logger.info("Downloading MovieLens-100k from %s", _DATA_URL)
        with urllib.request.urlopen(
            _DATA_URL, timeout=_NETWORK_TIMEOUT_SECONDS
        ) as resp:
            data = resp.read()
        with zipfile.ZipFile(io.BytesIO(data)) as z:
            z.extractall(cache_dir)
        _logger.info("Extracted MovieLens-100k to %s", cache_dir)
        return udata_path
    except (urllib.error.URLError, TimeoutError, OSError) as exc:
        _logger.warning(
            "Canonical URL failed (%s); falling back to %s", exc, _UDATA_MIRROR_URL
        )

    os.makedirs(extracted_dir, exist_ok=True)
    with urllib.request.urlopen(
        _UDATA_MIRROR_URL, timeout=_NETWORK_TIMEOUT_SECONDS
    ) as resp:
        udata_bytes = resp.read()
    with open(udata_path, "wb") as f:
        f.write(udata_bytes)
    _logger.info("Downloaded u.data mirror to %s", udata_path)
    return udata_path


def load_movielens_100k(
    cache_dir: str = _DEFAULT_CACHE_DIR,
    val_fraction: float = 0.2,
    seed: int = 42,
) -> MovieLensSplits:
    """Load MovieLens-100k and return train/val Ray datasets plus catalog sizes.

    The original ``user_id`` / ``item_id`` columns are non-contiguous integers;
    we remap them to dense 0-based indices so they can index :class:`torch.nn.Embedding`
    directly. ``rating`` is normalized from ``{1..5}`` to ``[0, 1]`` for a sigmoid head.
    """
    udata_path = _download_and_extract(cache_dir)

    df = pd.read_csv(
        udata_path,
        sep="\t",
        header=None,
        names=["user_id", "item_id", "rating", "timestamp"],
        dtype={
            "user_id": np.int64,
            "item_id": np.int64,
            "rating": np.int64,
            "timestamp": np.int64,
        },
    )
    _logger.info("Loaded %d ratings", len(df))

    user_id_to_idx = {
        uid: idx for idx, uid in enumerate(sorted(df["user_id"].unique()))
    }
    item_id_to_idx = {
        iid: idx for idx, iid in enumerate(sorted(df["item_id"].unique()))
    }
    df["user_idx"] = df["user_id"].map(user_id_to_idx).astype(np.int64)
    df["item_idx"] = df["item_id"].map(item_id_to_idx).astype(np.int64)
    df["rating_norm"] = ((df["rating"].astype(np.float32) - 1.0) / 4.0).astype(
        np.float32
    )

    num_users = len(user_id_to_idx)
    num_items = len(item_id_to_idx)
    _logger.info("Catalog: %d users, %d items", num_users, num_items)

    rng = np.random.default_rng(seed)
    perm = rng.permutation(len(df))
    cutoff = int(len(df) * (1.0 - val_fraction))
    train_df = df.iloc[perm[:cutoff]].reset_index(drop=True)
    val_df = df.iloc[perm[cutoff:]].reset_index(drop=True)
    _logger.info("Split: %d train / %d val", len(train_df), len(val_df))

    # Ray Train workers only need user_idx / item_idx / rating_norm.
    keep = ["user_idx", "item_idx", "rating_norm"]
    train_ds = ray.data.from_pandas(train_df[keep])
    val_ds = ray.data.from_pandas(val_df[keep])

    return MovieLensSplits(
        train=train_ds,
        val=val_ds,
        num_users=num_users,
        num_items=num_items,
    )
