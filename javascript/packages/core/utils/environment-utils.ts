const ENVIRONMENT_LABEL_KEY = 'pipelinerun.michelangelo/environment';

const ENVIRONMENT_LABEL_MAP: Record<string, 'Development' | 'Production' | 'Testing'> = {
  development: 'Development',
  production: 'Production',
  testing: 'Testing',
};

/**
 * Reads the CRD environment label and returns a human-readable environment name.
 *
 * CRD-backed entities have no dedicated `environment` field on their spec — the environment is
 * conveyed out-of-band via a `pipelinerun.michelangelo/environment` metadata label with raw
 * lowercase values (`development`, `production`, `testing`) — the same key the OSS apiserver
 * writes and that `pipelinerun_environment_label` in `mactl/config.py` documents. This normalizes
 * those raw values to the labels shown in the UI, and returns an empty string when the label is
 * absent or unrecognized (e.g. an environment value introduced after this map was last updated)
 * so the column renders blank rather than a raw label string.
 *
 * @example
 * readEnvironmentLabel({ 'pipelinerun.michelangelo/environment': 'production' }) // 'Production'
 * readEnvironmentLabel(undefined) // ''
 */
export function readEnvironmentLabel(
  labels?: Record<string, string>
): 'Development' | 'Production' | 'Testing' | '' {
  const raw = labels?.[ENVIRONMENT_LABEL_KEY];
  if (!raw) return '';
  return ENVIRONMENT_LABEL_MAP[raw.toLowerCase()] ?? '';
}
