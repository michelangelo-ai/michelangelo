import { getCrdUpdatedSeconds } from '../crd-utils';

describe('getCrdUpdatedSeconds', () => {
  test('prefers the SpecUpdateTimestamp label when present, converting microseconds to seconds', () => {
    expect(
      getCrdUpdatedSeconds({
        metadata: {
          labels: { 'michelangelo/SpecUpdateTimestamp': '1700000000000000' },
          creationTimestamp: '2022-04-15T05:20:00Z',
        },
      })
    ).toEqual(1700000000);
  });

  test('falls back to creationTimestamp when the label is absent', () => {
    expect(
      getCrdUpdatedSeconds({
        metadata: { creationTimestamp: '2022-04-15T05:20:00Z' },
      })
    ).toEqual(1650000000);
  });

  test('returns undefined when neither the label nor creationTimestamp is present', () => {
    expect(getCrdUpdatedSeconds({ metadata: {} })).toBeUndefined();
    expect(getCrdUpdatedSeconds({})).toBeUndefined();
  });
});
