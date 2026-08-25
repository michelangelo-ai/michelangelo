import { readEnvironmentLabel } from '../environment-utils';

describe('readEnvironmentLabel', () => {
  test('returns "Development" for development', () => {
    expect(readEnvironmentLabel({ 'pipelinerun.michelangelo/environment': 'development' })).toEqual(
      'Development'
    );
  });

  test('returns "Production" for production', () => {
    expect(readEnvironmentLabel({ 'pipelinerun.michelangelo/environment': 'production' })).toEqual(
      'Production'
    );
  });

  test('returns "Testing" for testing', () => {
    expect(readEnvironmentLabel({ 'pipelinerun.michelangelo/environment': 'testing' })).toEqual(
      'Testing'
    );
  });

  test('returns an empty string when the label is absent', () => {
    expect(readEnvironmentLabel(undefined)).toEqual('');
    expect(readEnvironmentLabel({})).toEqual('');
  });

  test('returns an empty string for an unrecognized raw value', () => {
    expect(readEnvironmentLabel({ 'pipelinerun.michelangelo/environment': 'staging' })).toEqual('');
  });
});
