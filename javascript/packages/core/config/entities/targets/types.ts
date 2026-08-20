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
  };
};
