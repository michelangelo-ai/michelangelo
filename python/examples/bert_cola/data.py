import logging

import datasets
import ray
import transformers
from ray.data import Dataset

import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask

tokenizer_path = "bert-base-cased"

log = logging.getLogger(__name__)


@uniflow.task(
    config=RayTask(
        head_cpu=1,
        head_memory="2Gi",
        worker_cpu=1,
        worker_memory="2Gi",
        worker_instances=1,
        # breakpoint=True,
    ),
    cache_enabled=False,
)
def load_data(
    path: str,
    name: str,
    tokenizer_max_length: int = 128,
) -> tuple[Dataset, Dataset, Dataset]:
    tokenizer = transformers.AutoTokenizer.from_pretrained(tokenizer_path)

    def tokenize_sentence(batch):
        outputs = tokenizer(
            batch["sentence"].tolist(),
            max_length=tokenizer_max_length,
            truncation=True,
            padding="max_length",
            return_tensors="np",
        )
        outputs["label"] = batch["label"]
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
