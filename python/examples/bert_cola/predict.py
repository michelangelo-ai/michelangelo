import requests
from transformers import AutoTokenizer

# Triton inference endpoint
url = "http://localhost:8080/v2/models/bert_cola/infer"

# Input text
text = "Example input for prediction."

# Tokenize input
tokenizer = AutoTokenizer.from_pretrained("bert-base-cased")
inputs = tokenizer(text, return_tensors="np")

input_ids = inputs["input_ids"].tolist()
attention_mask = inputs["attention_mask"].tolist()

payload = {
    "inputs": [
        {
            "name": "input_ids",
            "shape": [len(input_ids), len(input_ids[0])],
            "datatype": "INT64",
            "data": sum(input_ids, []),  # flatten the list
        },
        {
            "name": "attention_mask",
            "shape": [len(attention_mask), len(attention_mask[0])],
            "datatype": "INT64",
            "data": sum(attention_mask, []),  # flatten the list
        },
    ]
}

# Send inference request
response = requests.post(url, json=payload)

if response.status_code == 200:
    print("Prediction result:", response.json())
else:
    print("Inference failed:", response.text)
