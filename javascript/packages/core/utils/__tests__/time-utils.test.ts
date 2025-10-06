import { getDateFromEpochSeconds, getEpochSecondsFromDate, parseISOString } from '../time-utils';

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

  describe('parseISOString', () => {
    const validCases = [
      {
        input: '2024-01-01T12:00:00.000Z',
        expected: { date: '2024-01-01', time: '12:00:00.000Z' },
        description: 'standard ISO string with milliseconds',
      },
      {
        input: '2024-12-25T23:59:59Z',
        expected: { date: '2024-12-25', time: '23:59:59Z' },
        description: 'ISO string without milliseconds',
      },
      {
        input: '2024-02-29T00:00:00.000Z',
        expected: { date: '2024-02-29', time: '00:00:00.000Z' },
        description: 'leap year date',
      },
      {
        input: '1970-01-01T00:00:00.000Z',
        expected: { date: '1970-01-01', time: '00:00:00.000Z' },
        description: 'Unix epoch start',
      },
    ];

    const invalidCases = [
      { input: 'invalid-date', description: 'completely invalid string' },
      { input: '2024-13-01T12:00:00.000Z', description: 'invalid month' },
      { input: 'not-a-date', description: 'non-date string' },
      { input: '', description: 'empty string' },
      { input: '2024-01-01 12:00:00', description: 'space instead of T separator' },
      { input: '2024-01-01', description: 'date only, no time' },
      { input: '12:00:00', description: 'time only, no date' },
      { input: '2024-01-01T12:00:00T.000Z', description: 'multiple T separators' },
    ];

    test.each(validCases)('should parse $description', ({ input, expected }) => {
      const result = parseISOString(input);
      expect(result).toEqual(expected);
    });

    test.each(invalidCases)('should return null for $description', ({ input }) => {
      const result = parseISOString(input);
      expect(result).toBe(null);
    });
  });
});
