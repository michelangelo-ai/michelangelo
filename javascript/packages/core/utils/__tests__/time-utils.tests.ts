import { UserTimeZone } from '#core/providers/user-provider/types';
import { mockTimezone } from '#core/test/utils/mock-timezone';
import { timestampToString } from '../time-utils';

describe('time-utils', () => {
  describe('timestampToString', () => {
    const restore = mockTimezone();

    afterAll(() => {
      restore();
    });

    test('Handles falsy data', () => {
      // @ts-expect-error todo(ts-migration) TS2345 Argument of type 'null' is not assignable to parameter of type 'string | number'.
      expect(timestampToString(null)).toBeFalsy();
    });

    test('Handles non-epoch-second data', () => {
      expect(timestampToString('something invalid')).toEqual('Invalid date');
    });

    test('Formats epoch seconds into date with time', () => {
      expect(timestampToString(1671189655)).toMatch(
        // This format is any two-digit day within December 2022. Any time during that day.
        /2022\/12\/\d{2} \d{1,2}:\d{1,2}:\d{1,2}/
      );
    });

    test('formats date correctly without the timezone', () => {
      expect(timestampToString(1719907200)).toBe('2024/07/02 10:00:00 (GMT+2)');
      expect(timestampToString(1720656639)).toBe('2024/07/11 02:10:39 (GMT+2)');
      expect(timestampToString('1720656639')).toBe('2024/07/11 02:10:39 (GMT+2)');
    });

    test('formats date correctly in UTC timezone', () => {
      expect(timestampToString(1719907200, UserTimeZone.UTC)).toBe('2024/07/02 08:00:00 (UTC)');
      expect(timestampToString(1720656639, UserTimeZone.UTC)).toBe('2024/07/11 00:10:39 (UTC)');
      expect(timestampToString('1720656639', UserTimeZone.UTC)).toBe('2024/07/11 00:10:39 (UTC)');
    });

    test('formats date correctly in local timezone', () => {
      // GMT does not include summer time shift.
      expect(timestampToString(1705063132, UserTimeZone.Local)).toBe('2024/01/12 13:38:52 (GMT+1)');
      expect(timestampToString('1705063132', UserTimeZone.Local)).toBe(
        '2024/01/12 13:38:52 (GMT+1)'
      );

      // GMT includes summer time shift.
      expect(timestampToString(1719907200, UserTimeZone.Local)).toBe('2024/07/02 10:00:00 (GMT+2)');
      expect(timestampToString(1720656639, UserTimeZone.Local)).toBe('2024/07/11 02:10:39 (GMT+2)');
      expect(timestampToString('1720656639', UserTimeZone.Local)).toBe(
        '2024/07/11 02:10:39 (GMT+2)'
      );
    });
  });
});
