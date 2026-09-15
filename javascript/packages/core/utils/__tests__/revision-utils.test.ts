import { buildRevisionName, formatRevisionId } from '../revision-utils';

describe('revision-utils', () => {
  test('formatRevisionId truncates to the display length', () => {
    expect(formatRevisionId('3f2a1b9c0d4e5f6a7b8c')).toBe('Revision 3f2a1b9c0d4e');
    expect(formatRevisionId('short')).toBe('Revision short');
    expect(formatRevisionId(undefined)).toBe('');
  });

  test('buildRevisionName matches the controller naming scheme', () => {
    expect(buildRevisionName('pipeline', 'My-Pipeline', '3f2a1b9c0d4e5f6a7b8c')).toBe(
      'pipeline-my-pipeline-3f2a1b9c0d4e'
    );
    expect(buildRevisionName('pipeline', 'p', 'abc')).toBe('pipeline-p-abc');
  });
});
