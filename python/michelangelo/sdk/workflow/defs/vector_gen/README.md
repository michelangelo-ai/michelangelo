## Vector gen



### Build



```
bazel build //uber/ai/michelangelo/sdk/workflow/defs/vector_gen/...
```

### Also build any other packages you modified...

```
bazel build //uber/ai/michelangelo/sdk/workflow/tasks/document_loader/loader_plugins/...
```



### Run

#### test local pipeline



```
bazel run //uber/ai/michelangelo/projects/ma_dev_test_uber_one/pipelines/vector_dataset_gen_test:canvas_default_pipeline_local_run --strategy=local
```



#### run on MAStudio



```
ubuild build create -b <Branch> uber-one-michelangelo-library
```


Update the uber/ai/michelangelo/projects/ma_dev_test_uber_one/pipelines/vector_dataset_gen_test/pipeline.yaml with the new build id

#### Run Mactl
```
mactl pipeline apply -f uber/ai/michelangelo/projects/ma_dev_test_uber_one/pipelines/vector_dataset_gen_test/pipeline.yaml
 ```
