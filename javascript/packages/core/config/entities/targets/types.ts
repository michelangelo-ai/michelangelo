export type ClusterConnection = {
  host: string;
  port: string;
  tokenTag: string;
  caDataTag: string;
};

export type ClusterTarget = {
  clusterId: string;
  kubernetes: ClusterConnection;
};

export type InferenceServer = {
  metadata: {
    name: string;
    namespace: string;
  };
  spec: {
    tenancyType: string;
    backendType: string;
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

/** Subset of a `Cluster` CR needed to build a ClusterTarget. */
export type RegisteredCluster = {
  metadata: { name: string; namespace: string };
  spec?: {
    region?: string;
    zone?: string;
    kubernetes?: { rest?: ClusterConnection };
  };
};

/** A RegisteredCluster whose REST connection is present. */
export type ConnectableCluster = RegisteredCluster & {
  spec: { kubernetes: { rest: ClusterConnection } };
};

export type ClusterListResult = {
  clusterList: { items: RegisteredCluster[] };
};
