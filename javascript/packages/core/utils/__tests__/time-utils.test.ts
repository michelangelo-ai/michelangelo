import { getDateFromEpochSeconds, getEpochSecondsFromDate } from '../time-utils';

describe('time-utils', () => {
  describe('getDateFromEpochSeconds', () => {
    test('converts epoch seconds to Date object', () => {
      // January 1, 2024, 00:00:00 UTC
      const epochSeconds = 1704067200;
      const result = getDateFromEpochSeconds(epochSeconds);

      expect(result).toBeInstanceOf(Date);
      expect(result.getTime()).toBe(epochSeconds * 1000);
      expect(result.getUTCFullYear()).toBe(2024);
      expect(result.getUTCMonth()).toBe(0); // January is 0
      expect(result.getUTCDate()).toBe(1);
    });

    test('handles zero epoch seconds', () => {
      const result = getDateFromEpochSeconds(0);
      expect(result.getTime()).toBe(0);
      expect(result.toISOString()).toBe('1970-01-01T00:00:00.000Z');
    });

    test('handles negative epoch seconds', () => {
      const epochSeconds = -86400; // One day before Unix epoch
      const result = getDateFromEpochSeconds(epochSeconds);
      expect(result.getTime()).toBe(-86400000);
    });
  });

  describe('getEpochSecondsFromDate', () => {
    test('converts Date object to epoch seconds', () => {
      // January 1, 2024, 00:00:00 UTC
      const date = new Date('2024-01-01T00:00:00.000Z');
      const result = getEpochSecondsFromDate(date);

      expect(result).toBe(1704067200);
    });

    test('handles Unix epoch', () => {
      const date = new Date('1970-01-01T00:00:00.000Z');
      const result = getEpochSecondsFromDate(date);
      expect(result).toBe(0);
    });

    test('handles dates with milliseconds (rounds down)', () => {
      const date = new Date('2024-01-01T00:00:00.999Z');
      const result = getEpochSecondsFromDate(date);
      expect(result).toBe(1704067200); // Should floor the result
    });

    test('round trip conversion', () => {
      const originalEpochSeconds = 1704067200;
      const date = getDateFromEpochSeconds(originalEpochSeconds);
      const backToEpochSeconds = getEpochSecondsFromDate(date);

      expect(backToEpochSeconds).toBe(originalEpochSeconds);
    });
  });
});
