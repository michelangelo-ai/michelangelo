import { isEmptyFieldValue } from '#core/components/form/utils/is-empty-field-value';

describe('isEmptyFieldValue', () => {
  it.each([
    ['null', null, true],
    ['undefined', undefined, true],
    ['empty string', '', true],
    ['empty array', [], true],
    ['whitespace string', ' ', false],
    ['non-empty string', 'a', false],
    ['zero', 0, false],
    ['false', false, false],
    ['non-empty array', [1], false],
    ['empty object', {}, false],
    ['NaN', NaN, false],
  ] as const)('%s -> %s', (_description, value, expected) => {
    expect(isEmptyFieldValue(value)).toBe(expected);
  });
});
