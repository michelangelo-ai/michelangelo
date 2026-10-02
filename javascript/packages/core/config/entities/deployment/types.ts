export type ResourceRef = { name?: string; namespace?: string };

export type DeploymentCreateInput = {
  metadata: {
    name: string;
    namespace: string;
  };
  spec: {
    modelFamily: { name: string; namespace?: string };
    desiredRevision: { name: string; namespace?: string };
    inferenceServer: { name: string; namespace?: string };
    strategy: { rolling: { incrementPercentage: number } };
    definition: { type: string };
  };
};

export type DeploymentUpdateInput = {
  metadata: { name: string };
  spec: { desiredRevision?: ResourceRef; inferenceServer?: ResourceRef };
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

export type DeploymentRecord = {
  metadata?: {
    name?: string;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
  };
  spec?: {
    definition?: { type?: string };
    selector?: {
      matchLabels?: Record<string, string>;
      matchExpressions?: { values?: string[] }[];
    };
    strategy?: { rolling?: { incrementPercentage?: number } };
    inferenceServer?: ResourceRef;
    desiredRevision?: ResourceRef;
    modelFamily?: ResourceRef;
    resourceLinks?: Record<string, string>;
  };
  status?: {
    message?: string;
    stage?: string;
    currentRevision?: ResourceRef;
    candidateRevision?: ResourceRef;
  };
};
