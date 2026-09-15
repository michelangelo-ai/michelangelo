export type ClusterConnection = {
  host: string;
  port: string;
  tokenTag: string;
  caDataTag: string;
};

export type ClusterTarget = {
  clusterId: string;
  connection: { case: 'kubernetes'; value: ClusterConnection };
};

export type InferenceServer = {
  metadata: {
    name: string;
    namespace: string;
  };
  spec: {
    tenancyType: number;
    backendType: number;
    ownerSpec?: {
      ownerInfo?: {
        owningTeam?: string;
        owners?: string[];
        ownerGroups?: string[];
      };
      tier?: number;
    };
    initSpec: {
      resourceSpec: {
        cpu: number;
        memory: string;
        diskSize: string;
        gpu: number;
      };
      servingSpec?: {
        version?: string;
        containerBuildTemplate?: string;
      };
      numInstances: number;
    };
    clusterTargets?: ClusterTarget[];
  };
};

/**
 * Form-only shape for the create dialog. `clusterIds` holds the names picked from the
 * registered Cluster list and is mapped to `spec.clusterTargets` before submission.
 */
export type InferenceServerCreateInput = InferenceServer & {
  clusterIds: string[];
};

/** Subset of a `Cluster` CR (as decoded by protobuf-es) needed to build a ClusterTarget. */
export type RegisteredCluster = {
  metadata: { name: string; namespace: string };
  spec?: {
    region?: string;
    zone?: string;
    cluster?: { case?: 'kubernetes'; value?: { rest?: Partial<ClusterConnection> } };
  };
};

export type ClusterListResult = {
  clusterList: { items: RegisteredCluster[] };
};
