## Remote Run

 poetry run python ./examples/bert_cola/bert_cola.py  remote-run --image docker.io/library/examples:latest --storage-url s3://default --yes

### get the predictor pod

```bash
kubectl get pods -n default
```
Example returns

```bash
NAME                                                              READY   STATUS      RESTARTS   AGE
bert-cola-deployment-predictor-00001-deployment-7d9c5b8797s9cgd   2/2     Running     0          119s
````
### Port forwarding to the predictor pod
```bash
kubectl port-forward deploy/bert-cola-deployment-predictor-00001-deployment 8080:8080 
```

### Test the predictor
```bash

curl -X POST http://localhost:8080/v2/models/bert-cola/infer \
-H "Content-Type: application/json" \
-d '{
  "inputs": [
    {
      "name": "input_ids",
      "shape": [1, 10],
      "datatype": "INT64",
      "data": [101, 7592, 999, 102, 0, 0, 0, 0, 0, 0]
    },
    {
      "name": "attention_mask",
      "shape": [1, 10],
      "datatype": "INT64",
      "data": [1, 1, 1, 1, 0, 0, 0, 0, 0, 0]
    }
  ]
}'
```
