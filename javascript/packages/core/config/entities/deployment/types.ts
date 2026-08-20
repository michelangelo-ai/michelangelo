export type ResourceRef = { name?: string; namespace?: string };

export type DeploymentCreateInput = {
  metadata: {
    name: string;
    namespace: string;
  };
  spec: {
    desiredRevision: { name: string; namespace?: string };
    target: {
      case: 'inferenceServer';
      value: { name: string; namespace?: string };
    };
    strategy: {
      rolloutStrategy: {
        case: 'rolling';
        value: { incrementPercentage: number };
      };
    };
    definition: { type: number };
  };
};

export type InferenceServerListResult = {
  inferenceServerList: {
    items: Array<{ metadata: { name: string } }>;
  };
};

export type ModelFamilyListResult = {
  modelFamilyList: {
    items: Array<{ metadata: { name: string }; spec: { name: string } }>;
  };
};

export type ModelListResult = {
  modelList: {
    items: Array<{ metadata: { name: string } }>;
  };
};

export type RevisionListResult = {
  revisionList: {
    items: Array<{
      metadata: { name: string; creationTimestamp?: { toDate: () => Date } };
      spec: { baseResource?: { name: string } };
    }>;
  };
};

export type DeploymentRecord = {
  metadata?: {
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
  };
  spec?: {
    definition?: { type?: number };
    selector?: {
      matchLabels?: Record<string, string>;
      matchExpressions?: { values?: string[] }[];
    };
    strategy?: { rolloutStrategy?: { case?: string } };
    target?: { case?: string; value?: ResourceRef };
    desiredRevision?: ResourceRef;
    resourceLinks?: Record<string, string>;
  };
  status?: {
    message?: string;
    stage?: number;
    currentRevision?: ResourceRef;
    candidateRevision?: ResourceRef;
  };
};
