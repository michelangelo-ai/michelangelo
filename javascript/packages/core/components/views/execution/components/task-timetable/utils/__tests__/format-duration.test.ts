import { formatDuration } from '../format-duration';

describe('formatDuration', () => {
  it.each([
    [0, '0s'],
    [12, '12s'],
    [105, '1m 45s'],
    [120, '2m'],
    [7500, '2h 5m'],
    [97200, '1d 3h'],
    [-5, '0s'],
  ])('should format %i seconds as %s', (durationSeconds, expected) => {
    expect(formatDuration(durationSeconds)).toBe(expected);
  });
});
