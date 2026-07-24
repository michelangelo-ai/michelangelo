"""Real (non-mocked) integration tests for V2 auto-resume.

These exercise the actual redesign end-to-end on a local Ray cluster — no
mocking of the resume path — to guard against the class of bug where the unit
suite stays green while the feature is broken against the installed Ray runtime
(the original PR called the deprecated ``TorchTrainer.can_restore`` here, which
crashes on Ray 2.51.x). Two scenarios are covered:

* Same-identity re-run: a second run with the same ``RunConfig(storage_path,
  name)`` must not crash and must resume from the first run's checkpoint (Ray
  Train V2's native snapshot restoration).
* Cross-directory seed: a run whose own directory has no native checkpoint
  resumes from a checkpoint located by a custom ``ExperimentStore`` pointing at
  a different run's directory (the store's load-bearing role in V2).

They are slow (~1-2 min total) because each ``train()`` spins up Ray workers,
and each spawns a real Ray cluster. On memory-constrained CI runners (e.g.
GitHub-hosted, ~7 GB RAM / 8 GB ``/dev/shm``) ``ray.init()`` OOM-kills the
pytest process — taking down the whole suite — so they are **skipped in CI by
default**. Run them locally (the default off-CI), or in a dedicated
Ray-enabled CI job, by setting ``MICHELANGELO_RUN_RAY_INTEGRATION_TESTS=1``.
"""

from __future__ import annotations

import os
import sys

import pytest

# Skip in CI unless explicitly forced (see module docstring): a real Ray cluster
# exceeds the memory of shared coverage runners and OOM-kills the whole run.
if (os.environ.get("CI") or os.environ.get("GITHUB_ACTIONS")) and (
    os.environ.get("MICHELANGELO_RUN_RAY_INTEGRATION_TESTS") != "1"
):
    pytest.skip(
        "Skipping real-Ray integration tests in CI (they spin up a Ray cluster "
        "and OOM on constrained runners). Set "
        "MICHELANGELO_RUN_RAY_INTEGRATION_TESTS=1 to run them.",
        allow_module_level=True,
    )

pytest.importorskip("ray")
torch = pytest.importorskip("torch")
pytest.importorskip("pytorch_lightning")

import pytorch_lightning as pl  # noqa: E402
import ray  # noqa: E402
import ray.cloudpickle as cloudpickle  # noqa: E402
import ray.train  # noqa: E402
import torch.nn as nn  # noqa: E402
from pytorch_lightning.callbacks import ModelCheckpoint  # noqa: E402

from michelangelo.lib.trainer.torch.pytorch_lightning import (  # noqa: E402
    LightningTrainer,
    LightningTrainerParam,
)
from michelangelo.lib.trainer.torch.pytorch_lightning.experiment_store import (  # noqa: E402
    FsspecExperimentStore,
)

# The model factory and classes below are defined in this test module. Ray
# workers cannot import the test module by reference (it is not part of the
# installed package), so serialize objects defined here by value instead.
cloudpickle.register_pickle_by_value(sys.modules[__name__])


class _TinyRegressor(pl.LightningModule):
    """Minimal Lightning module: a single linear layer trained on toy data."""

    def __init__(self) -> None:
        """Build the one-layer regressor."""
        super().__init__()
        self.net = nn.Linear(2, 1)

    def training_step(self, batch, batch_idx):
        """Report and return MSE on a batch."""
        x = batch["x"].float()
        y = batch["y"].float().view(-1, 1)
        loss = ((self.net(x) - y) ** 2).mean()
        self.log("train_loss", loss)
        return loss

    def validation_step(self, batch, batch_idx):
        """Report validation MSE on a batch."""
        x = batch["x"].float()
        y = batch["y"].float().view(-1, 1)
        self.log("val_loss", ((self.net(x) - y) ** 2).mean())

    def configure_optimizers(self):
        """Use plain SGD so optimizer state round-trips through checkpoints."""
        return torch.optim.SGD(self.parameters(), lr=0.01)


def _make_model() -> _TinyRegressor:
    """Top-level factory (picklable) constructing the toy model on each worker."""
    return _TinyRegressor()


class _PointerStore:
    """Custom ``ExperimentStore`` whose ``locate_resumable`` returns a fixed dir.

    Picklable (holds only a string), so it survives transport to Ray workers.
    Used to prove the store can seed a resume from a *different* run's directory.
    """

    def __init__(self, target: str | None) -> None:
        """Store the experiment directory to hand back from ``locate_resumable``."""
        self._target = target

    def track(self, *, storage_path: str, run_name: str, experiment_path: str) -> None:
        """No-op: this store does not persist anything."""

    def locate_resumable(self, *, storage_path: str, run_name: str) -> str | None:
        """Return the fixed target directory (or ``None``)."""
        return self._target


@pytest.fixture(scope="module")
def ray_cluster():
    """Start a tiny local Ray cluster for the duration of the module."""
    ray.init(num_cpus=2, include_dashboard=False, ignore_reinit_error=True)
    try:
        yield
    finally:
        ray.shutdown()


def _train(storage_path, name, epochs, store):
    """Run one training job and return its result dict."""
    dataset = ray.data.from_items(
        [{"x": [float(i), float(i + 1)], "y": float(i)} for i in range(8)]
    )
    param = LightningTrainerParam(
        create_model_fn=_make_model,
        create_model_fn_kwargs={},
        train_data=dataset,
        val_data=dataset,
        batch_size=4,
        lightning_trainer_kwargs={
            "max_epochs": epochs,
            "callbacks": [ModelCheckpoint(save_top_k=2, monitor="val_loss")],
        },
        experiment_store=store,
    )
    trainer = LightningTrainer(
        trainer_param=param,
        run_config=ray.train.RunConfig(name=name, storage_path=str(storage_path)),
        scaling_config=ray.train.ScalingConfig(num_workers=1, use_gpu=False),
    )
    return trainer.train()


class TestAutoResumeIntegration:
    """End-to-end auto-resume against a real Ray cluster."""

    def test_same_identity_rerun_resumes_without_crash(self, ray_cluster, tmp_path):
        """A second run with the same identity resumes past the first run's epoch.

        This is the exact scenario the pre-fix code crashed on (it called the
        deprecated ``can_restore`` when a marker was present). Success means the
        re-run neither crashes nor restarts from scratch.
        """
        store = FsspecExperimentStore()
        first = _train(tmp_path, "resume_run", epochs=1, store=store)
        assert first["metrics"]["epoch"] == 0

        # Re-run with the same identity and more epochs: must resume, not crash.
        second = _train(tmp_path, "resume_run", epochs=3, store=store)
        assert second["metrics"]["epoch"] == 2  # continued past epoch 0

    def test_store_seeds_resume_from_other_run_directory(self, ray_cluster, tmp_path):
        """A fresh-directory run resumes from a checkpoint the store points at.

        Run A trains and produces checkpoints in ``run_a/``. Run B uses a new
        name (its own directory has no native Ray checkpoint) with a custom
        store pointing at ``run_a/``; the worker must seed from run A's latest
        checkpoint and continue rather than restart.
        """
        run_a = _train(tmp_path, "run_a", epochs=2, store=_PointerStore(None))
        assert run_a["metrics"]["epoch"] == 1

        run_a_dir = str(tmp_path / "run_a")
        run_b = _train(tmp_path, "run_b", epochs=4, store=_PointerStore(run_a_dir))
        # Seeded from run A (which ended at epoch 1 / step 4) and continued.
        assert run_b["metrics"]["epoch"] == 3
        assert run_b["metrics"]["step"] > run_a["metrics"]["step"]
