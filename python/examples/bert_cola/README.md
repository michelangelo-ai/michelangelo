## Prerequisites
1. Create Sandbox through `ma sandbox create`

2.a. This pipeline uses MLFLOW, hence the following steps are required:
    1. run `kubectl exec -it mysql -- bash` on terminal to exec into the mysql pod
    2. `mysql -u root -p`
    3. enter "root" for password
    4. run `create database mlflow_db;`
    5. then exit the pod

2.b. Create a bucket called `mlflow` in minio

3.a. create pipeline task image by running:
```bash
docker build -t examples:latest -f ./examples/Dockerfile .
```
3.b. import image into cluster by running 
```bash
k3d image import examples:latest -c michelangelo-sandbox
```


## Run pipeline on Cadence

```bash
PYTHONPATH="." poetry run python ./examples/bert_cola/bert_cola.py  remote-run --image docker.io/library/examples:latest --storage-url s3://default --yes
```