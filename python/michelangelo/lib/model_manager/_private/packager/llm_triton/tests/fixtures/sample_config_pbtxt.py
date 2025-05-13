SAMPLE_CONFIG_PBTXT = """\
name: "model-0"
backend: "python"
max_batch_size: 256
dynamic_batching: {
  preferred_batch_size: 10,
  max_queue_delay_microseconds: 300,
  preserve_ordering: true
}
input : [
  {
    name: "prompt",
    data_type: TYPE_STRING,
    dims: [ 1 ]
  },
  {
    name: "max_tokens",
    data_type: TYPE_INT32,
    dims: [ 1 ]
  },
  {
    name: "top_p",
    data_type: TYPE_FP32,
    dims: [ 1 ]
  },
  {
    name: "temperature",
    data_type: TYPE_FP32,
    dims: [ 1 ]
  },
  {
    name: "n",
    data_type: TYPE_INT32,
    dims: [ 1 ]
  }
]
output: [
  {
    name: "text_json",
    data_type: TYPE_STRING,
    dims: [ 1 ]
  },
  {
    name: "response",
    data_type: TYPE_STRING,
    dims: [ 1 ]
  }
]
instance_group: [
  {
    kind: KIND_CPU,
    count: 1
  }
]
"""
