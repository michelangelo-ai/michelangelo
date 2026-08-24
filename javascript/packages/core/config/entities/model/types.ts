/** Shape of a `Model` record as read by the Model detail page's custom tab components. */
export type ModelRecord = {
  metadata?: { name?: string };
  spec?: {
    description?: string;
    sourcePipelineRun?: { name?: string };
    performanceEvaluationReport?: { name?: string };
  };
};

/** An evaluation report resource, referenced by a model's Performance tab. */
export type EvaluationReportRecord = {
  spec?: {
    title?: string;
    charts?: ChartRecord[];
  };
};

/**
 * One chart in an evaluation report. Only `title` is used today — rendering a chart's actual
 * content (as a table, line chart, histogram, etc., depending on its type) is a separate,
 * not-yet-implemented piece of work.
 */
export type ChartRecord = {
  title?: string;
};
