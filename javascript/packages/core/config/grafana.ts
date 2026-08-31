// Default matches how Prometheus/Grafana/the UI itself are already consumed in the sandbox
// (localhost port-forwards); override via setGrafanaBaseUrl for non-sandbox environments.
let grafanaBaseUrl = 'http://localhost:13000';

export function setGrafanaBaseUrl(url: string): void {
  grafanaBaseUrl = url;
}

export function getGrafanaBaseUrl(): string {
  return grafanaBaseUrl;
}

/**
 * Deep-links to the `controllermgr` Grafana dashboard (uid: adgpdn8), filtered to a single
 * pipeline run via its `pipeline_run` template variable. Only controllermgr reconcile metrics
 * (success/error rate, timing) are labeled by pipeline_run today — there is no per-job Ray
 * metric (CPU/memory/GPU) to filter by.
 */
export function buildPipelineRunMetricsUrl(pipelineRunName: string): string {
  const params = new URLSearchParams({
    'var-pipeline_run': pipelineRunName,
    from: 'now-24h',
    to: 'now',
  });
  return `${getGrafanaBaseUrl()}/d/adgpdn8/controllermgr?${params.toString()}`;
}
