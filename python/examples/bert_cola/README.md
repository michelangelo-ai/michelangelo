## Steps

### Start Sandbox
  ma sandbox create --exclude apiserver controllermgr worker
  bazel run apiserver
  bazel run controllermgr
  bazel run worker
  ma sandbox demo

  Perform Port Foward

### Create folder in Minio
create those buckets in Minio
```
default
mlflow
deploy-models
```
### Create mysql db
```bash
kubectl exec -it mysql -- bash

mysql> mysql -u root -p
mysql> CREATE DATABASE mlflow_db;
```

## Remote Run
  docker build -t examples:latest -f ./examples/Dockerfile .
  k3d image import examples:latest -c michelangelo-sandbox
  PYTHONPATH="." poetry run python ./examples/bert_cola/bert_cola.py  remote-run --image docker.io/library/examples:latest --storage-url s3://default --yes

<!-- ### install kserve
```bash
 curl -s "https://raw.githubusercontent.com/kserve/kserve/release-0.15/hack/quick_install.sh" | bash
``` -->

<!-- ### get the predictor pod

```bash
kubectl apply -f michelangelo/cli/sandbox/resources/deployment.yaml
kubectl apply -f michelangelo/cli/sandbox/resources/secret.yaml
kubectl get pods -n default
```

Example returns

```bash
NAME                                                              READY   STATUS      RESTARTS   AGE
bert-cola-deployment-predictor-00001-deployment-7d9c5b8797s9cgd   2/2     Running     0          119s
```` -->

<!-- ### Port forwarding to the predictor pod
```bash
kubectl port-forward deploy/bert-cola-deployment-predictor-00001-deployment 8080:8080 
``` -->

### Test the predictor
```bash

curl -X POST http://localhost:8889/inference-server-bert-cola/v2/models/bert-cola-32/infer \
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
