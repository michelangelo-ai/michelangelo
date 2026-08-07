export type ResourceRef = { name?: string; namespace?: string };

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
