import logging

import datasets
import ray
import transformers
from ray.data import Dataset

from michelangelo.uniflow.plugins.ray import RayTask
import michelangelo.uniflow.core as uniflow


tokenizer_path = "bert-base-cased"

log = logging.getLogger(__name__)


@uniflow.task(config=RayTask(
    head_cpu=1,
    head_memory="2Gi",
    worker_cpu=1,
    worker_memory="2Gi",
    worker_instances=1))
def load_data(
    path: str,
    name: str,
    tokenizer_max_length: int = 128,
) -> tuple[Dataset, Dataset, Dataset]:
    tokenizer = transformers.AutoTokenizer.from_pretrained(tokenizer_path, trust_remote_code=True)

    def tokenize_sentence(batch):
        print("Batch Keys:", batch.keys())
        outputs = tokenizer(
            batch["text"].tolist(),
            max_length=tokenizer_max_length,
            truncation=True,
            padding="max_length",
            return_tensors="np",
        )
        outputs["labels"] = outputs["input_ids"].copy()
        return outputs

    data = datasets.load_dataset(path=path, name=name)

    def _load_slice(data_slice) -> Dataset:
        ds = ray.data.from_huggingface(data[data_slice])
        ds = ds.map_batches(tokenize_sentence, batch_format="numpy")

        ds = ds.random_sample(0.01, seed=1)

        return ds

    return (
        _load_slice("train"),
        _load_slice("validation"),
        _load_slice("test"),
    )
