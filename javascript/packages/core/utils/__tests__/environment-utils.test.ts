import { readEnvironmentLabel } from '../environment-utils';

describe('readEnvironmentLabel', () => {
  test('returns "Development" for ENV_TYPE_DEVELOPMENT', () => {
    expect(readEnvironmentLabel({ 'michelangelo/environment': 'ENV_TYPE_DEVELOPMENT' })).toEqual(
      'Development'
    );
  });

  test('returns "Production" for ENV_TYPE_PRODUCTION', () => {
    expect(readEnvironmentLabel({ 'michelangelo/environment': 'ENV_TYPE_PRODUCTION' })).toEqual(
      'Production'
    );
  });

  test('returns an empty string when the label is absent', () => {
    expect(readEnvironmentLabel(undefined)).toEqual('');
    expect(readEnvironmentLabel({})).toEqual('');
  });

  test('returns an empty string for an unrecognized raw value', () => {
    expect(readEnvironmentLabel({ 'michelangelo/environment': 'ENV_TYPE_STAGING' })).toEqual('');
  });
});
