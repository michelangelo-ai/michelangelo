import { buildRevisionName, formatRevisionLabel } from '../revision-utils';

describe('revision-utils', () => {
  test('formatRevisionLabel truncates to the display length', () => {
    expect(formatRevisionLabel('3f2a1b9c0d4e5f6a7b8c')).toBe('Revision 3f2a1b9c0d4e');
    expect(formatRevisionLabel('short')).toBe('Revision short');
    expect(formatRevisionLabel(undefined)).toBe('');
  });

  test('buildRevisionName matches the pipeline controller naming scheme', () => {
    expect(buildRevisionName('pipeline', 'My-Pipeline', '3f2a1b9c0d4e5f6a7b8c')).toBe(
      'pipeline-my-pipeline-3f2a1b9c0d4e'
    );
    expect(buildRevisionName('pipeline', 'p', 'abc')).toBe('pipeline-p-abc');
  });

  test('buildRevisionName falls back to entityId for an unrecognized service', () => {
    expect(buildRevisionName('model', 'My-Model', '3f2a1b9c0d4e5f6a7b8c')).toBe('My-Model');
  });
});
