load('/Users/baoquan/Documents/dev/oss/michelangelo/python/michelangelo/uniflow/plugins/ray/task.star', __ray_task='task')

def train_workflow():
    data_path = 'glue'
    data_name = 'cola'
    (train_data, validation_data, test_data) = __ray_task('examples.bert_cola.data.load_data', head_cpu=1, head_memory='2Gi', worker_cpu=1, worker_memory='2Gi', worker_instances=1, cache_enabled=False, cache_version=None)(data_path, data_name, tokenizer_max_length=128)
    result = __ray_task('examples.bert_cola.train.train', head_cpu=1, head_memory='4Gi', worker_cpu=1, worker_memory='4Gi', worker_instances=1, cache_enabled=False, cache_version=None)(train_data, validation_data, test_data)
    print('result:', result)
    print('ok.')