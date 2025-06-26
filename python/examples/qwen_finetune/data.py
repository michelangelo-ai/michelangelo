import logging
import datasets
import ray
import transformers
from ray.data import Dataset
from michelangelo.uniflow.plugins.ray import RayTask
import michelangelo.uniflow.core as uniflow

tokenizer_path = "Qwen/Qwen1.5-1.8B-Chat"

log = logging.getLogger(__name__)


@uniflow.task(
    config=RayTask(
        head_cpu=1,
        head_memory="4Gi",
        worker_cpu=1,
        worker_memory="4Gi",
        worker_instances=1,
    ),
    cache_enabled=True,
)
def load_data(
    dataset_name: str = "squad",
    tokenizer_max_length: int = 512,
) -> tuple[Dataset, Dataset, Dataset]:
    """Load and preprocess data for Qwen fine-tuning"""
    
    tokenizer = transformers.AutoTokenizer.from_pretrained(tokenizer_path)
    
    # Add pad token if it doesn't exist
    if tokenizer.pad_token is None:
        tokenizer.pad_token = tokenizer.eos_token

    def format_instruction(example):
        """Format data as instruction-following examples for Qwen"""
        if dataset_name == "squad":
            # Format SQuAD as Q&A
            question = example["question"]
            context = example["context"]
            
            # Handle different answer formats
            answers = example.get("answers", {})
            if isinstance(answers, dict):
                answer_texts = answers.get("text", [])
                if isinstance(answer_texts, list) and len(answer_texts) > 0:
                    answer = answer_texts[0]
                elif isinstance(answer_texts, str):
                    answer = answer_texts
                else:
                    answer = "No answer"
            else:
                answer = "No answer"
            
            instruction = f"Answer the following question based on the context.\n\nContext: {context}\n\nQuestion: {question}\n\nAnswer:"
            response = f" {answer}"
            
        else:
            # Default format for other datasets
            instruction = example.get("instruction", example.get("text", ""))
            response = example.get("response", example.get("label", ""))
        
        return {
            "instruction": instruction,
            "response": response,
            "input_text": instruction + response
        }

    def tokenize_function(batch):
        """Tokenize the formatted examples"""
        # Handle different batch formats
        if isinstance(batch, dict):
            # Extract examples from batch dictionary format
            if "question" in batch and isinstance(batch["question"], list):
                # Batch is in columnar format (question: [q1, q2, ...], context: [c1, c2, ...])
                batch_size = len(batch["question"])
                examples = []
                for i in range(batch_size):
                    example = {}
                    for key in batch:
                        example[key] = batch[key][i]
                    examples.append(example)
            else:
                # Single example in dict format
                examples = [batch]
        else:
            # Batch is a list of examples
            examples = batch
        
        # Format instructions
        formatted_examples = [format_instruction(example) for example in examples]
        
        # Extract input texts
        input_texts = [ex["input_text"] for ex in formatted_examples]
        
        # Tokenize
        tokenized = tokenizer(
            input_texts,
            max_length=tokenizer_max_length,
            truncation=True,
            padding="max_length",
            return_tensors="np",
        )
        
        # For causal LM, labels are the same as input_ids
        tokenized["labels"] = tokenized["input_ids"].copy()
        
        return tokenized

    # Load dataset
    if dataset_name == "squad":
        try:
            data = datasets.load_dataset("squad")
        except Exception as e:
            log.warning(f"Failed to load SQuAD dataset: {e}")
            log.info("Falling back to synthetic data for demonstration")
            # Create synthetic Q&A data for demonstration
            synthetic_data = []
            for i in range(100):
                synthetic_data.append({
                    "question": f"What is example question {i}?",
                    "context": f"This is example context {i} that contains relevant information for the question.",
                    "answers": {"text": [f"answer {i}"], "answer_start": [0]}
                })
            
            # Create a dataset-like structure
            class SyntheticDataset:
                def __init__(self, data):
                    self.data = data
                def __getitem__(self, key):
                    return {"train": self.data, "validation": self.data[:20]}
            
            data = SyntheticDataset(synthetic_data)
    else:
        # Default to a simple text dataset
        data = datasets.load_dataset("wikitext", "wikitext-2-raw-v1")

    def _load_slice(data_slice) -> Dataset:
        try:
            ds = ray.data.from_huggingface(data[data_slice])
        except Exception as e:
            log.warning(f"Failed to load slice '{data_slice}' from HuggingFace: {e}")
            log.info("Using synthetic data instead")
            # Create synthetic data directly as Ray dataset
            synthetic_samples = []
            for i in range(100):
                synthetic_samples.append({
                    "question": f"What is example question {i}?",
                    "context": f"This is example context {i} that contains relevant information for the question.",
                    "answers": {"text": [f"answer {i}"], "answer_start": [0]}
                })
            ds = ray.data.from_items(synthetic_samples)
        
        ds = ds.map_batches(tokenize_function, batch_format="numpy")
        
        # Sample a small subset for demonstration
        ds = ds.random_sample(0.01, seed=42)
        
        return ds

    # Handle different dataset splits
    if dataset_name == "squad":
        train_ds = _load_slice("train")
        validation_ds = _load_slice("validation")
        # SQuAD doesn't have a test set, so we'll use validation for test
        test_ds = validation_ds
    else:
        train_ds = _load_slice("train")
        validation_ds = _load_slice("validation")
        test_ds = _load_slice("test")

    return train_ds, validation_ds, test_ds