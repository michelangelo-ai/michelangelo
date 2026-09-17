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

/** Subset of a `Cluster` CR (as decoded by protobuf-es) needed to build a ClusterTarget. */
export type RegisteredCluster = {
  metadata: { name: string; namespace: string };
  spec?: {
    region?: string;
    zone?: string;
    cluster?: { case?: 'kubernetes'; value?: { rest?: ClusterConnection } };
  };
};

/** A RegisteredCluster whose REST connection is present. */
export type ConnectableCluster = RegisteredCluster & {
  spec: { cluster: { value: { rest: ClusterConnection } } };
};

export type ClusterListResult = {
  clusterList: { items: RegisteredCluster[] };
};
