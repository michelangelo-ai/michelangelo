import { interpolate } from '#core/interpolation/interpolate';
import { clearUnresolvedInterpolations } from '../clear-unresolved-interpolations';

describe('clearUnresolvedInterpolations', () => {
  test.each([
    {
      description: 'object with no interpolations returns unchanged',
      input: { key: 'value' },
      expected: { key: 'value' },
    },
    {
      description: 'object with interpolation property clears interpolation to undefined',
      input: { key: interpolate('abc-123') },
      expected: { key: undefined },
    },
    {
      description: 'object with nested interpolation clears nested interpolation to undefined',
      input: { key: { nestedKey: interpolate('abc-123') } },
      expected: { key: { nestedKey: undefined } },
    },
    {
      description: 'object with array containing interpolation leaves array unchanged',
      input: { key: [1, interpolate('abc-123')] },
      expected: { key: [1, interpolate('abc-123')] },
    },
    {
      description: 'object with null value returns null unchanged',
      input: { key: null },
      expected: { key: null },
    },
    {
      description: 'object with undefined value returns undefined unchanged',
      input: { key: undefined },
      expected: { key: undefined },
    },
    {
      description: 'object with mixed content clears only object interpolations',
      input: { mixed: 'value', clear: interpolate('${data}'), keep: [interpolate('${array}')] },
      expected: { mixed: 'value', clear: undefined, keep: [interpolate('${array}')] },
    },
  ])('$description', ({ input, expected }) => {
    expect(clearUnresolvedInterpolations(input)).toEqual(expected);
  });
});
