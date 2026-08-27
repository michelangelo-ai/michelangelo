export type ModelRecord = {
  metadata?: {
    labels?: Record<string, string>;
    creationTimestamp?: { seconds: number };
  };
  spec?: {
    owner?: { name?: string };
    description?: string;
    kind?: number;
    sourcePipelineRun?: { name?: string; namespace?: string };
    qualityScores?: { name?: string; value?: number }[];
    modelFamily?: { name?: string; namespace?: string };
    trainingFramework?: string;
    source?: string;
    predictionResult?: {
      trainTableName?: string;
      testTableName?: string;
    };
  };
};
