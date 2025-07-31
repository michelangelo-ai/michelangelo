// Successful pipeline run execution data
export const successfulPipelineRun = {
  status: {
    steps: [
      {
        subSteps: [
          {
            subSteps: [],
            resources: [],
            attemptIds: [],
            name: 'build-20250730-235746-5015cad3',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            startTime: {
              seconds: '1753919866',
              nanos: 321261718,
            },
            endTime: {
              seconds: '1753920437',
              nanos: 239845110,
            },
            logUrl: 'https://buildkite.com/uber/ubuild-hightier-global/builds/18791463',
            output: {
              fields: {
                image: {
                  stringValue:
                    'ml-uber-ai-michelangelo-experimental-maf-tasks-feature-prep:bkt1-produ-1753919867-05494',
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
            name: 'build-20250730-235746-912cdb9a',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            startTime: {
              seconds: '1753919866',
              nanos: 355842634,
            },
            endTime: {
              seconds: '1753920437',
              nanos: 250926515,
            },
            logUrl: 'https://buildkite.com/uber/ubuild-hightier-global/builds/18791464',
            output: {
              fields: {
                image: {
                  stringValue:
                    'ml-uber-ai-michelangelo-experimental-maf-tasks-preprocess:bkt1-produ-1753919867-e1f05',
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
            name: 'build-20250730-235746-bd09a957',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            startTime: {
              seconds: '1753919866',
              nanos: 500441632,
            },
            endTime: {
              seconds: '1753920437',
              nanos: 424683216,
            },
            logUrl: 'https://buildkite.com/uber/ubuild-hightier-global/builds/18791465',
            output: {
              fields: {
                image: {
                  stringValue:
                    'ml-uber-ai-michelangelo-experimental-maf-tasks-train:bkt1-produ-1753919867-31b03',
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
          seconds: '1753919866',
          nanos: 295131259,
        },
        endTime: {
          seconds: '1753920437',
          nanos: 424685937,
        },
        logUrl: '',
        output: {
          fields: {
            preprocess: {
              stringValue:
                'ml-uber-ai-michelangelo-experimental-maf-tasks-preprocess:bkt1-produ-1753919867-e1f05',
              kind: 'stringValue',
            },
            feature_prep: {
              stringValue:
                'ml-uber-ai-michelangelo-experimental-maf-tasks-feature-prep:bkt1-produ-1753919867-05494',
              kind: 'stringValue',
            },
            train: {
              stringValue:
                'ml-uber-ai-michelangelo-experimental-maf-tasks-train:bkt1-produ-1753919867-31b03',
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
                  url: 'https://compute.uberinternal.com/clusters/dca60-kubernetes-batch01/jobs/ma-ray-uniflow-ray-2t8dq',
                },
                resource: 'externalResource',
              },
            ],
            attemptIds: ['1'],
            name: 'uber.ai.michelangelo.experimental.maf.tasks.feature_prep.task.feature_prep',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            startTime: {
              seconds: '1753920437',
              nanos: 0,
            },
            endTime: {
              seconds: '1753920632',
              nanos: 0,
            },
            logUrl:
              'https://compute.uberinternal.com/clusters/dca60-kubernetes-batch01/jobs/ma-ray-uniflow-ray-2t8dq',
            output: null,
            message: '',
            displayName: 'feature_prep',
            input: null,
            stepCachedOutputs: {
              intermediateVars: [
                {
                  namespace: 'ma-dev-test-uber-one',
                  name: 'uf-vars-l4cvj',
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
                  url: 'https://compute.uberinternal.com/clusters/dca60-kubernetes-batch01/jobs/spark-operator-spark-1863733-455bc6c8193f42c2b6bb87584ba081bb/tasks/0/sandbox',
                },
                resource: 'externalResource',
              },
            ],
            attemptIds: ['1'],
            name: 'uber.ai.michelangelo.experimental.maf.tasks.preprocess.task.preprocess',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            startTime: {
              seconds: '1753920632',
              nanos: 0,
            },
            endTime: {
              seconds: '1753920755',
              nanos: 0,
            },
            logUrl:
              'https://compute.uberinternal.com/clusters/dca60-kubernetes-batch01/jobs/spark-operator-spark-1863733-455bc6c8193f42c2b6bb87584ba081bb/tasks/0/sandbox',
            output: null,
            message: '',
            displayName: 'preprocess',
            input: null,
            stepCachedOutputs: {
              intermediateVars: [
                {
                  namespace: 'ma-dev-test-uber-one',
                  name: 'uf-vars-bknwj',
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
                  url: 'https://compute.uberinternal.com/clusters/dca60-kubernetes-batch01/jobs/ma-ray-uniflow-ray-nzd8q',
                },
                resource: 'externalResource',
              },
            ],
            attemptIds: ['1'],
            name: 'uber.ai.michelangelo.experimental.maf.tasks.train.task.train',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            startTime: {
              seconds: '1753920755',
              nanos: 0,
            },
            endTime: {
              seconds: '1753920941',
              nanos: 0,
            },
            logUrl:
              'https://compute.uberinternal.com/clusters/dca60-kubernetes-batch01/jobs/ma-ray-uniflow-ray-nzd8q',
            output: null,
            message: '',
            displayName: 'train',
            input: null,
            stepCachedOutputs: {
              intermediateVars: [
                {
                  namespace: 'ma-dev-test-uber-one',
                  name: 'uf-vars-ckdk7',
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
          seconds: '1753920437',
          nanos: 774150202,
        },
        endTime: {
          seconds: '1753920947',
          nanos: 936570090,
        },
        logUrl:
          'https://cadence-v4.uberinternal.com/domains/michelangelo-controllermgr/prod17-phx/workflows/run-20250730-235745-8e3d601c/7696ef4f-c5e6-416d-8cda-dd0baa012375/summary',
        output: null,
        message: '',
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
            name: 'build-20250731-060048-898e6aed',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            startTime: {
              seconds: '1753941648',
              nanos: 251373923,
            },
            endTime: {
              seconds: '1753941716',
              nanos: 862493439,
            },
            logUrl: 'https://buildkite.com/uber/ubuild-hightier-global/builds/18791921',
            output: {
              fields: {
                image: {
                  stringValue:
                    'ml-uber-ai-michelangelo-sdk-workflow-tasks-document-loader:bkt1-produ-1753925563-ca8d8',
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
            name: 'build-20250731-060048-6c45386a',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            startTime: {
              seconds: '1753941648',
              nanos: 273014353,
            },
            endTime: {
              seconds: '1753941716',
              nanos: 998031127,
            },
            logUrl: 'https://buildkite.com/uber/ubuild-hightier-global/builds/18791922',
            output: {
              fields: {
                image: {
                  stringValue:
                    'ml-uber-ai-michelangelo-sdk-workflow-tasks-llm-inference:bkt1-produ-1753925563-b49c1',
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
            name: 'build-20250731-060048-830881d7',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            startTime: {
              seconds: '1753941648',
              nanos: 294432748,
            },
            endTime: {
              seconds: '1753941717',
              nanos: 17992753,
            },
            logUrl: 'https://buildkite.com/uber/ubuild-hightier-global/builds/18791923',
            output: {
              fields: {
                image: {
                  stringValue:
                    'ml-uber-ai-michelangelo-sdk-workflow-tasks-pusher:bkt1-produ-1753925563-f5679',
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
            name: 'build-20250731-060048-ee18362e',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            startTime: {
              seconds: '1753941648',
              nanos: 432988555,
            },
            endTime: {
              seconds: '1753941717',
              nanos: 204937725,
            },
            logUrl: 'https://buildkite.com/uber/ubuild-hightier-global/builds/18791923',
            output: {
              fields: {
                image: {
                  stringValue:
                    'ml-uber-ai-michelangelo-sdk-workflow-tasks-pusher:bkt1-produ-1753925563-f5679',
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
            name: 'build-20250731-060048-5eb65222',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            startTime: {
              seconds: '1753941648',
              nanos: 561761482,
            },
            endTime: {
              seconds: '1753941717',
              nanos: 215243125,
            },
            logUrl: 'https://buildkite.com/uber/ubuild-hightier-global/builds/18791923',
            output: {
              fields: {
                image: {
                  stringValue:
                    'ml-uber-ai-michelangelo-sdk-workflow-tasks-pusher:bkt1-produ-1753925563-f5679',
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
          seconds: '1753941648',
          nanos: 104111002,
        },
        endTime: {
          seconds: '1753941717',
          nanos: 215247124,
        },
        logUrl: '',
        output: {
          fields: {
            pusher: {
              stringValue:
                'ml-uber-ai-michelangelo-sdk-workflow-tasks-pusher:bkt1-produ-1753925563-f5679',
              kind: 'stringValue',
            },
            inference: {
              stringValue:
                'ml-uber-ai-michelangelo-sdk-workflow-tasks-llm-inference:bkt1-produ-1753925563-b49c1',
              kind: 'stringValue',
            },
            document_loader: {
              stringValue:
                'ml-uber-ai-michelangelo-sdk-workflow-tasks-document-loader:bkt1-produ-1753925563-ca8d8',
              kind: 'stringValue',
            },
            pusher_online: {
              stringValue:
                'ml-uber-ai-michelangelo-sdk-workflow-tasks-pusher:bkt1-produ-1753925563-f5679',
              kind: 'stringValue',
            },
            pusher_offline: {
              stringValue:
                'ml-uber-ai-michelangelo-sdk-workflow-tasks-pusher:bkt1-produ-1753925563-f5679',
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
                  url: 'https://compute.uberinternal.com/clusters/dca60-kubernetes-batch01/jobs/spark-operator-spark-1865427-6a6d5cec049e4086a094b70b917c3f76/tasks/0/sandbox',
                },
                resource: 'externalResource',
              },
            ],
            attemptIds: ['1'],
            name: 'uber.ai.michelangelo.sdk.workflow.tasks.document_loader.task.document_loader',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            startTime: {
              seconds: '1753941725',
              nanos: 0,
            },
            endTime: {
              seconds: '1753941889',
              nanos: 0,
            },
            logUrl:
              'https://compute.uberinternal.com/clusters/dca60-kubernetes-batch01/jobs/spark-operator-spark-1865427-6a6d5cec049e4086a094b70b917c3f76/tasks/0/sandbox',
            output: null,
            message: '',
            displayName: 'document_loader',
            input: null,
            stepCachedOutputs: {
              intermediateVars: [
                {
                  namespace: 'ma-dev-test-uber-one',
                  name: 'uf-vars-hw5nt',
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
                  url: 'https://compute.uberinternal.com/clusters/dca60-kubernetes-batch01/jobs/ma-ray-uniflow-ray-4dclm',
                },
                resource: 'externalResource',
              },
            ],
            attemptIds: ['1'],
            name: 'uber.ai.michelangelo.sdk.workflow.tasks.llm_inference.task.llm_inference',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            startTime: {
              seconds: '1753941889',
              nanos: 0,
            },
            endTime: {
              seconds: '1753942114',
              nanos: 0,
            },
            logUrl:
              'https://compute.uberinternal.com/clusters/dca60-kubernetes-batch01/jobs/ma-ray-uniflow-ray-4dclm',
            output: null,
            message: '',
            displayName: 'inference',
            input: null,
            stepCachedOutputs: {
              intermediateVars: [
                {
                  namespace: 'ma-dev-test-uber-one',
                  name: 'uf-vars-frttk',
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
                  url: 'https://compute.uberinternal.com/clusters/dca60-kubernetes-batch01/jobs/spark-operator-spark-1865483-9469a67e640448c9994103807e0835c3/tasks/0/sandbox',
                },
                resource: 'externalResource',
              },
            ],
            attemptIds: ['1'],
            name: 'uber.ai.michelangelo.sdk.workflow.tasks.pusher.task.pusher',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            startTime: {
              seconds: '1753942114',
              nanos: 0,
            },
            endTime: {
              seconds: '1753942297',
              nanos: 0,
            },
            logUrl:
              'https://compute.uberinternal.com/clusters/dca60-kubernetes-batch01/jobs/spark-operator-spark-1865483-9469a67e640448c9994103807e0835c3/tasks/0/sandbox',
            output: null,
            message: '',
            displayName: 'pusher_offline',
            input: null,
            stepCachedOutputs: {
              intermediateVars: [
                {
                  namespace: 'ma-dev-test-uber-one',
                  name: 'uf-vars-z7p84',
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
                  url: 'https://compute.uberinternal.com/clusters/dca60-kubernetes-batch01/jobs/spark-operator-spark-1865509-26f4d0ce60e5473c9926dc022178a921/tasks/0/sandbox',
                },
                resource: 'externalResource',
              },
            ],
            attemptIds: ['1'],
            name: 'uber.ai.michelangelo.sdk.workflow.tasks.pusher.task.pusher',
            state: 'PIPELINE_RUN_STEP_STATE_FAILED',
            startTime: {
              seconds: '1753942298',
              nanos: 0,
            },
            endTime: {
              seconds: '1753942420',
              nanos: 0,
            },
            logUrl:
              'https://compute.uberinternal.com/clusters/dca60-kubernetes-batch01/jobs/spark-operator-spark-1865509-26f4d0ce60e5473c9926dc022178a921/tasks/0/sandbox',
            output: null,
            message: 'unknown reason:Spark job failed',
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
          seconds: '1753941725',
          nanos: 902375100,
        },
        endTime: {
          seconds: '1753942435',
          nanos: 408308457,
        },
        logUrl:
          'https://cadence-v4.uberinternal.com/domains/michelangelo-controllermgr/prod17-phx/workflows/run-20250731-060000-32d0c231/6f17350e-a95e-4592-a9f4-b1f205ea08c4/summary',
        output: null,
        message: 'Cadence Workflow Execution Close Status: FAILED',
        displayName: 'Execute Workflow',
        input: null,
        stepCachedOutputs: null,
      },
    ],
  },
};
