import logging

import datasets
import ray
import transformers
from ray.data import Dataset

# from uber.ai.uniflow.ray_task import Ray
import michelangelo.uniflow as uniflow


tokenizer_path = "bert-base-cased"

log = logging.getLogger(__name__)


@uniflow.task(
    config="test"
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

        if uniflow.is_local_run():
            ds = ds.random_sample(0.2, seed=1)

        return ds

    return (
        _load_slice("train"),
        _load_slice("validation"),
        _load_slice("test"),
    )
