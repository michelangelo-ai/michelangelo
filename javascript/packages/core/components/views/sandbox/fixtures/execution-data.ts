// Successful pipeline run execution data
export const successfulPipelineRun = {
  status: {
    steps: [
      {
        subSteps: [],
        displayName: 'Data Validation',
        state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
        startTime: { seconds: '1704067000', nanos: 0 },
        endTime: { seconds: '1704067100', nanos: 0 },
        message:
          'Successfully validated dataset with 1000 valid rows and 5 invalid rows. Schema validation passed.',
        input: {
          fields: {
            dataset_path: { stringValue: '/data/training.csv', kind: 'stringValue' },
            validation_rules: { stringValue: 'schema_v1.json', kind: 'stringValue' },
          },
        },
        output: {
          fields: {
            valid_rows: { numberValue: 1000, kind: 'numberValue' },
            invalid_rows: { numberValue: 5, kind: 'numberValue' },
          },
        },
        stepCachedOutputs: {
          intermediateVars: [{ namespace: 'ml-workspace', name: 'validation-cache-123' }],
        },
        logUrl: 'https://example.com/logs/data-validation',
      },
      {
        subSteps: [
          {
            subSteps: [],
            resources: [],
            attemptIds: [],
            name: 'build-20240101-120000-abc123',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            startTime: {
              seconds: '1704067200',
              nanos: 321261718,
            },
            endTime: {
              seconds: '1704067800',
              nanos: 239845110,
            },
            logUrl: 'https://example.com/builds/12345',
            output: {
              fields: {
                image: {
                  stringValue: 'ml-feature-prep:v1.0.0-abc123',
                  kind: 'stringValue',
                },
              },
            },
            message: '',
            displayName: 'feature_prep',
            input: null,
            stepCachedOutputs: null,
          },
          {
            subSteps: [],
            resources: [],
            attemptIds: [],
            name: 'build-20240101-120000-def456',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            startTime: {
              seconds: '1704067200',
              nanos: 355842634,
            },
            endTime: {
              seconds: '1704067800',
              nanos: 250926515,
            },
            logUrl: 'https://example.com/builds/12346',
            output: {
              fields: {
                image: {
                  stringValue: 'ml-preprocess:v1.0.0-def456',
                  kind: 'stringValue',
                },
              },
            },
            message: '',
            displayName: 'preprocess',
            input: null,
            stepCachedOutputs: null,
          },
          {
            subSteps: [],
            resources: [],
            attemptIds: [],
            name: 'build-20240101-120000-ghi789',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            startTime: {
              seconds: '1704067200',
              nanos: 500441632,
            },
            endTime: {
              seconds: '1704067800',
              nanos: 424683216,
            },
            logUrl: 'https://example.com/builds/12347',
            output: {
              fields: {
                image: {
                  stringValue: 'ml-train:v1.0.0-ghi789',
                  kind: 'stringValue',
                },
              },
            },
            message: '',
            displayName: 'train',
            input: null,
            stepCachedOutputs: null,
          },
        ],
        resources: [],
        attemptIds: [],
        name: 'Image Build',
        state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
        startTime: {
          seconds: '1704067200',
          nanos: 295131259,
        },
        endTime: {
          seconds: '1704067800',
          nanos: 424685937,
        },
        logUrl: 'https://build.example.com/logs/image-build-20240101',
        message: 'All container images built successfully. Ready for deployment phase.',
        output: {
          fields: {
            preprocess: {
              stringValue: 'ml-preprocess:v1.0.0-def456',
              kind: 'stringValue',
            },
            feature_prep: {
              stringValue: 'ml-feature-prep:v1.0.0-abc123',
              kind: 'stringValue',
            },
            train: {
              stringValue: 'ml-train:v1.0.0-ghi789',
              kind: 'stringValue',
            },
          },
        },
        displayName: 'Image Build',
        input: null,
        stepCachedOutputs: null,
      },
      {
        subSteps: [
          {
            subSteps: [],
            resources: [
              {
                externalResource: {
                  name: 'Attempt1-DriverURL',
                  url: 'https://compute.example.com/clusters/batch-cluster/jobs/feature-prep-job-123',
                },
                resource: 'externalResource',
              },
            ],
            attemptIds: ['1'],
            name: 'ml.tasks.feature_prep.task.feature_prep',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            startTime: {
              seconds: '1704067800',
              nanos: 0,
            },
            endTime: {
              seconds: '1704068000',
              nanos: 0,
            },
            logUrl: 'https://compute.example.com/clusters/batch-cluster/jobs/feature-prep-job-123',
            output: null,
            message: '',
            displayName: 'feature_prep',
            input: null,
            stepCachedOutputs: {
              intermediateVars: [
                {
                  namespace: 'ml-dev-workspace',
                  name: 'vars-abc123',
                },
              ],
              checkpoints: [],
              rawModels: [],
            },
          },
          {
            subSteps: [],
            resources: [
              {
                externalResource: {
                  name: 'Attempt1-DriverURL',
                  url: 'https://compute.example.com/clusters/batch-cluster/jobs/preprocess-job-456',
                },
                resource: 'externalResource',
              },
            ],
            attemptIds: ['1'],
            name: 'ml.tasks.preprocess.task.preprocess',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            startTime: {
              seconds: '1704068000',
              nanos: 0,
            },
            endTime: {
              seconds: '1704068200',
              nanos: 0,
            },
            logUrl: 'https://compute.example.com/clusters/batch-cluster/jobs/preprocess-job-456',
            output: null,
            message: '',
            displayName: 'preprocess',
            input: null,
            stepCachedOutputs: {
              intermediateVars: [
                {
                  namespace: 'ml-dev-workspace',
                  name: 'vars-def456',
                },
              ],
              checkpoints: [],
              rawModels: [],
            },
          },
          {
            subSteps: [],
            resources: [
              {
                externalResource: {
                  name: 'Attempt1-DriverURL',
                  url: 'https://compute.example.com/clusters/batch-cluster/jobs/train-job-789',
                },
                resource: 'externalResource',
              },
            ],
            attemptIds: ['1'],
            name: 'ml.tasks.train.task.train',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            startTime: {
              seconds: '1704068200',
              nanos: 0,
            },
            endTime: {
              seconds: '1704068400',
              nanos: 0,
            },
            logUrl: 'https://compute.example.com/clusters/batch-cluster/jobs/train-job-789',
            output: null,
            message: '',
            displayName: 'train',
            input: null,
            stepCachedOutputs: {
              intermediateVars: [
                {
                  namespace: 'ml-dev-workspace',
                  name: 'vars-ghi789',
                },
              ],
              checkpoints: [],
              rawModels: [],
            },
          },
        ],
        resources: [],
        attemptIds: [],
        name: 'Execute Workflow',
        state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
        startTime: {
          seconds: '1704067800',
          nanos: 774150202,
        },
        endTime: {
          seconds: '1704068400',
          nanos: 936570090,
        },
        logUrl: 'https://workflow.example.com/executions/run-20240101-120000/summary',
        output: null,
        message: 'Workflow completed successfully. All tasks executed without errors.',
        displayName: 'Execute Workflow',
        input: null,
        stepCachedOutputs: null,
      },
    ],
  },
};

// Failed pipeline run execution data
export const failurePipelineRun = {
  status: {
    steps: [
      {
        subSteps: [
          {
            subSteps: [],
            resources: [],
            attemptIds: [],
            name: 'build-20240101-130000-jkl012',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            startTime: {
              seconds: '1704070800',
              nanos: 251373923,
            },
            endTime: {
              seconds: '1704071400',
              nanos: 862493439,
            },
            logUrl: 'https://example.com/builds/12348',
            output: {
              fields: {
                image: {
                  stringValue: 'ml-document-loader:v1.0.0-jkl012',
                  kind: 'stringValue',
                },
              },
            },
            message: '',
            displayName: 'document_loader',
            input: null,
            stepCachedOutputs: null,
          },
          {
            subSteps: [],
            resources: [],
            attemptIds: [],
            name: 'build-20240101-130000-mno345',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            startTime: {
              seconds: '1704070800',
              nanos: 273014353,
            },
            endTime: {
              seconds: '1704071400',
              nanos: 998031127,
            },
            logUrl: 'https://example.com/builds/12349',
            output: {
              fields: {
                image: {
                  stringValue: 'ml-inference:v1.0.0-mno345',
                  kind: 'stringValue',
                },
              },
            },
            message: '',
            displayName: 'inference',
            input: null,
            stepCachedOutputs: null,
          },
          {
            subSteps: [],
            resources: [],
            attemptIds: [],
            name: 'build-20240101-130000-pqr678',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            startTime: {
              seconds: '1704070800',
              nanos: 294432748,
            },
            endTime: {
              seconds: '1704071400',
              nanos: 17992753,
            },
            logUrl: 'https://example.com/builds/12350',
            output: {
              fields: {
                image: {
                  stringValue: 'ml-pusher:v1.0.0-pqr678',
                  kind: 'stringValue',
                },
              },
            },
            message: '',
            displayName: 'pusher_offline',
            input: null,
            stepCachedOutputs: null,
          },
          {
            subSteps: [],
            resources: [],
            attemptIds: [],
            name: 'build-20240101-130000-stu901',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            startTime: {
              seconds: '1704070800',
              nanos: 432988555,
            },
            endTime: {
              seconds: '1704071400',
              nanos: 204937725,
            },
            logUrl: 'https://example.com/builds/12351',
            output: {
              fields: {
                image: {
                  stringValue: 'ml-pusher:v1.0.0-stu901',
                  kind: 'stringValue',
                },
              },
            },
            message: '',
            displayName: 'pusher_online',
            input: null,
            stepCachedOutputs: null,
          },
          {
            subSteps: [],
            resources: [],
            attemptIds: [],
            name: 'build-20240101-130000-vwx234',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            startTime: {
              seconds: '1704070800',
              nanos: 561761482,
            },
            endTime: {
              seconds: '1704071400',
              nanos: 215243125,
            },
            logUrl: 'https://example.com/builds/12352',
            output: {
              fields: {
                image: {
                  stringValue: 'ml-pusher:v1.0.0-vwx234',
                  kind: 'stringValue',
                },
              },
            },
            message: '',
            displayName: 'pusher',
            input: null,
            stepCachedOutputs: null,
          },
        ],
        resources: [],
        attemptIds: [],
        name: 'Image Build',
        state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
        startTime: {
          seconds: '1704070800',
          nanos: 104111002,
        },
        endTime: {
          seconds: '1704071400',
          nanos: 215247124,
        },
        logUrl: '',
        output: {
          fields: {
            pusher: {
              stringValue: 'ml-pusher:v1.0.0-vwx234',
              kind: 'stringValue',
            },
            inference: {
              stringValue: 'ml-inference:v1.0.0-mno345',
              kind: 'stringValue',
            },
            document_loader: {
              stringValue: 'ml-document-loader:v1.0.0-jkl012',
              kind: 'stringValue',
            },
            pusher_online: {
              stringValue: 'ml-pusher:v1.0.0-stu901',
              kind: 'stringValue',
            },
            pusher_offline: {
              stringValue: 'ml-pusher:v1.0.0-pqr678',
              kind: 'stringValue',
            },
          },
        },
        message: '',
        displayName: 'Image Build',
        input: null,
        stepCachedOutputs: null,
      },
      {
        subSteps: [
          {
            subSteps: [],
            resources: [
              {
                externalResource: {
                  name: 'Attempt1-DriverURL',
                  url: 'https://compute.example.com/clusters/batch-cluster/jobs/document-loader-job-123',
                },
                resource: 'externalResource',
              },
            ],
            attemptIds: ['1'],
            name: 'ml.tasks.document_loader.task.document_loader',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            startTime: {
              seconds: '1704071400',
              nanos: 0,
            },
            endTime: {
              seconds: '1704071600',
              nanos: 0,
            },
            logUrl:
              'https://compute.example.com/clusters/batch-cluster/jobs/document-loader-job-123',
            output: null,
            message: '',
            displayName: 'document_loader',
            input: null,
            stepCachedOutputs: {
              intermediateVars: [
                {
                  namespace: 'ml-dev-workspace',
                  name: 'vars-jkl012',
                },
              ],
              checkpoints: [],
              rawModels: [],
            },
          },
          {
            subSteps: [],
            resources: [
              {
                externalResource: {
                  name: 'Attempt1-DriverURL',
                  url: 'https://compute.example.com/clusters/batch-cluster/jobs/inference-job-456',
                },
                resource: 'externalResource',
              },
            ],
            attemptIds: ['1'],
            name: 'ml.tasks.inference.task.inference',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            startTime: {
              seconds: '1704071600',
              nanos: 0,
            },
            endTime: {
              seconds: '1704071800',
              nanos: 0,
            },
            logUrl: 'https://compute.example.com/clusters/batch-cluster/jobs/inference-job-456',
            output: null,
            message: '',
            displayName: 'inference',
            input: null,
            stepCachedOutputs: {
              intermediateVars: [
                {
                  namespace: 'ml-dev-workspace',
                  name: 'vars-mno345',
                },
              ],
              checkpoints: [],
              rawModels: [],
            },
          },
          {
            subSteps: [],
            resources: [
              {
                externalResource: {
                  name: 'Attempt1-DriverURL',
                  url: 'https://compute.example.com/clusters/batch-cluster/jobs/pusher-offline-job-789',
                },
                resource: 'externalResource',
              },
            ],
            attemptIds: ['1'],
            name: 'ml.tasks.pusher.task.pusher',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            startTime: {
              seconds: '1704071800',
              nanos: 0,
            },
            endTime: {
              seconds: '1704072000',
              nanos: 0,
            },
            logUrl:
              'https://compute.example.com/clusters/batch-cluster/jobs/pusher-offline-job-789',
            output: null,
            message: '',
            displayName: 'pusher_offline',
            input: null,
            stepCachedOutputs: {
              intermediateVars: [
                {
                  namespace: 'ml-dev-workspace',
                  name: 'vars-pqr678',
                },
              ],
              checkpoints: [],
              rawModels: [],
            },
          },
          {
            subSteps: [],
            resources: [
              {
                externalResource: {
                  name: 'Attempt1-DriverURL',
                  url: 'https://compute.example.com/clusters/batch-cluster/jobs/pusher-online-job-012',
                },
                resource: 'externalResource',
              },
            ],
            attemptIds: ['1'],
            name: 'ml.tasks.pusher.task.pusher',
            state: 'PIPELINE_RUN_STEP_STATE_FAILED',
            startTime: {
              seconds: '1704072000',
              nanos: 0,
            },
            endTime: {
              seconds: '1704072200',
              nanos: 0,
            },
            logUrl: 'https://compute.example.com/clusters/batch-cluster/jobs/pusher-online-job-012',
            output: null,
            message: 'Task execution failed: Connection timeout',
            displayName: 'pusher_online',
            input: null,
            stepCachedOutputs: null,
          },
        ],
        resources: [],
        attemptIds: [],
        name: 'Execute Workflow',
        state: 'PIPELINE_RUN_STEP_STATE_FAILED',
        startTime: {
          seconds: '1704071400',
          nanos: 902375100,
        },
        endTime: {
          seconds: '1704072200',
          nanos: 408308457,
        },
        logUrl: 'https://workflow.example.com/executions/run-20240101-130000/summary',
        output: null,
        message: 'Workflow execution failed',
        displayName: 'Execute Workflow',
        input: null,
        stepCachedOutputs: null,
      },
    ],
  },
};
