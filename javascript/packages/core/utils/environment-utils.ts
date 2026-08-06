const ENVIRONMENT_LABEL_KEY = 'michelangelo/environment';

const ENVIRONMENT_LABEL_MAP: Record<string, 'Development' | 'Production' | 'Testing'> = {
  development: 'Development',
  production: 'Production',
  testing: 'Testing',
};

/**
 * Reads the CRD environment label and returns a human-readable environment name.
 *
 * CRD-backed entities have no dedicated `environment` field on their spec — the environment is
 * conveyed out-of-band via a `michelangelo/environment` metadata label with raw lowercase values
 * (`development`, `production`, `testing`). This normalizes those raw values to the labels shown
 * in the UI, and returns an empty string when the label is absent or unrecognized (e.g. an
 * environment value introduced after this map was last updated) so the column renders blank
 * rather than a raw label string. (An earlier version of this function matched a prefixed
 * `ENV_TYPE_*` form that turned out not to match any real label value; this was corrected after
 * review, along with adding the `testing` case.)
 *
 * @example
 * readEnvironmentLabel({ 'michelangelo/environment': 'production' }) // 'Production'
 * readEnvironmentLabel(undefined) // ''
 */
export function readEnvironmentLabel(
  labels?: Record<string, string>
): 'Development' | 'Production' | 'Testing' | '' {
  const raw = labels?.[ENVIRONMENT_LABEL_KEY];
  if (!raw) return '';
  return ENVIRONMENT_LABEL_MAP[raw.toLowerCase()] ?? '';
}
