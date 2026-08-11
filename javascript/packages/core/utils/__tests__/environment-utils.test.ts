import { readEnvironmentLabel } from '../environment-utils';

describe('readEnvironmentLabel', () => {
  test('returns "Development" for development', () => {
    expect(readEnvironmentLabel({ 'michelangelo/environment': 'development' })).toEqual(
      'Development'
    );
  });

  test('returns "Production" for production', () => {
    expect(readEnvironmentLabel({ 'michelangelo/environment': 'production' })).toEqual(
      'Production'
    );
  });

  test('returns "Testing" for testing', () => {
    expect(readEnvironmentLabel({ 'michelangelo/environment': 'testing' })).toEqual('Testing');
  });

  test('returns an empty string when the label is absent', () => {
    expect(readEnvironmentLabel(undefined)).toEqual('');
    expect(readEnvironmentLabel({})).toEqual('');
  });

  test('returns an empty string for an unrecognized raw value', () => {
    expect(readEnvironmentLabel({ 'michelangelo/environment': 'staging' })).toEqual('');
  });
});
