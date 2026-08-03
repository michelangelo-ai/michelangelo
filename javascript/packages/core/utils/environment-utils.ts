const ENVIRONMENT_LABEL_KEY = 'michelangelo/environment';

/**
 * Reads the CRD environment label and returns a human-readable environment name.
 *
 * CRD-backed entities have no dedicated `environment` field on their spec — the environment is
 * conveyed out-of-band via a `michelangelo/environment` metadata label with raw values like
 * `ENV_TYPE_DEVELOPMENT` / `ENV_TYPE_PRODUCTION`. This normalizes those raw values to the labels
 * shown in the UI, and returns an empty string when the label is absent or unrecognized (e.g. an
 * environment value introduced after this map was last updated) so the column renders blank
 * rather than a raw enum string.
 *
 * @example
 * readEnvironmentLabel({ 'michelangelo/environment': 'ENV_TYPE_PRODUCTION' }) // 'Production'
 * readEnvironmentLabel(undefined) // ''
 */
export function readEnvironmentLabel(
  labels?: Record<string, string>
): 'Development' | 'Production' | '' {
  const raw = labels?.[ENVIRONMENT_LABEL_KEY];
  if (raw === 'ENV_TYPE_DEVELOPMENT') return 'Development';
  if (raw === 'ENV_TYPE_PRODUCTION') return 'Production';
  return '';
}
