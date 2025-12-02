import os

import tempfile
import unittest

import numpy as np
import torch

from torch.utils.data import DataLoader
import pytorch_lightning as pl

from michelangelo.canvas.lib.model.tte.pl_tte_model import (
    create_model,
    TwoTowerPLModule,
)

dir_path = os.path.dirname(os.path.realpath(__file__))
os.environ["TOKENIZERS_PARALLELISM"] = "false"


class TestSharedInBatchTwoTower(unittest.TestCase):
    def test_train_val_step(self):
        pl_tte_model = TwoTowerPLModule(
            tte_class_name="SharedInBatchTwoTower",
            text_model_class_name="TextEncoder",
            query_text_column="query",
            item_text_column="item",
            query_text_max_len=16,
            item_text_max_len=16,
            pretrained_text_model_path=os.path.join(dir_path, "unit_test_model_files"),
        )
        batch = {"query": ["hello me", "yes I am"], "item": ["hello me", "yes I am"]}
        loss = pl_tte_model._train_val_step(batch, True)
        pl_tte_model.validation_step(batch)
        pl_tte_model.test_step(batch)

        # test log_metrics
        log = {"train_loss": loss.detach(), "val_loss": loss.detach()}
        pl_tte_model.log_metrics(log, True)
        pl_tte_model.log_metrics(log, False)

        # test model saving
        tmp_model_dir = tempfile.TemporaryDirectory().name
        pl_tte_model.save_final_model(tmp_model_dir)
        assert loss.item() > 0.0

    def test_conf_optimizer(self):
        pl_tte_model = TwoTowerPLModule(
            tte_class_name="SharedInBatchTwoTower",
            text_model_class_name="TextEncoder",
            query_text_column="query",
            item_text_column="item",
            query_text_max_len=16,
            item_text_max_len=16,
            pretrained_text_model_path=os.path.join(dir_path, "unit_test_model_files"),
            optimizer_str="adam",
        )
        pl_tte_model.configure_optimizers()

        pl_tte_model = TwoTowerPLModule(
            tte_class_name="SharedInBatchTwoTower",
            text_model_class_name="TextEncoder",
            query_text_column="query",
            item_text_column="item",
            query_text_max_len=16,
            item_text_max_len=16,
            pretrained_text_model_path=os.path.join(dir_path, "unit_test_model_files"),
            optimizer_str="sgd",
        )
        pl_tte_model.configure_optimizers()

    def test_create_model(self):
        pl_tte_model = create_model(
            tte_class_name="SharedInBatchTwoTower",
            text_model_class_name="TextEncoder",
            query_text_column="query",
            item_text_column="item",
            query_text_max_len=8,
            item_text_max_len=16,
            pretrained_text_model_path=os.path.join(dir_path, "unit_test_model_files"),
        )
        batch = {"query": ["hello me", "yes I am"], "item": ["hello me", "yes I am"]}
        loss = pl_tte_model._train_val_step(batch, True)
        assert loss.item() > 0.0

    def test_model_train(self):
        pl_tte_model = create_model(
            tte_class_name="SharedInBatchTwoTower",
            text_model_class_name="TextEncoder",
            query_text_column="query",
            item_text_column="item",
            query_text_max_len=8,
            item_text_max_len=16,
            pretrained_text_model_path=os.path.join(dir_path, "unit_test_model_files"),
        )
        data = [{"query": "hello", "item": "me"} for i in range(20)]

        def collate_fn_to_torch(batch):
            new_batch = {
                "query": [],
                "item": [],
            }
            for d in batch:
                new_batch["query"].append(d["query"])
                new_batch["item"].append(d["item"])

            if "label" in new_batch:
                labels = new_batch["label"]
            else:
                labels = np.ones(len(new_batch["query"]))

            new_batch["label"] = torch.from_numpy(labels).float()
            return new_batch

        train_dataloader = DataLoader(
            data[:12],
            batch_size=2,
            collate_fn=collate_fn_to_torch,
        )
        val_dataloader = DataLoader(
            data[12:],
            batch_size=2,
            collate_fn=collate_fn_to_torch,
        )

        trainer = pl.Trainer(max_epochs=1)
        trainer.fit(pl_tte_model, train_dataloader, val_dataloader)
