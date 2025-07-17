import { interpolate } from '#core/interpolation/interpolate';
import { hasInterpolationProperty } from '../has-interpolation-property';

describe('hasInterpolationProperty', () => {
  test.each([
    {
      description: 'object with interpolation property has interpolation',
      input: { title: interpolate('${page.name}'), count: 42 },
      expected: true,
    },
    {
      description: 'nested object with interpolation has interpolation',
      input: { user: { email: interpolate('${user.email}') } },
      expected: true,
    },
    {
      description: 'array with interpolation has interpolation',
      input: ['static text', interpolate('${user.email}')],
      expected: true,
    },
    {
      description: 'deeply nested object with interpolation has interpolation',
      input: { metadata: { nested: { value: interpolate('${data.value}') } } },
      expected: true,
    },
    {
      description: 'object with array containing interpolation has interpolation',
      input: { items: [{ name: interpolate('${item.name}') }] },
      expected: true,
    },
    {
      description: 'direct interpolation instance has interpolation',
      input: interpolate('${page.title}'),
      expected: true,
    },
    {
      description: 'plain object has no interpolation',
      input: { name: 'John', age: 30 },
      expected: false,
    },
    {
      description: 'array with no interpolation has no interpolation',
      input: ['static', 'text', 'only'],
      expected: false,
    },
    {
      description: 'nested object with no interpolation has no interpolation',
      input: { nested: { values: ['no', 'interpolations'] } },
      expected: false,
    },
    {
      description: 'plain string has no interpolation',
      input: 'No interpolation',
      expected: false,
    },
    {
      description: 'number has no interpolation',
      input: 42,
      expected: false,
    },
    {
      description: 'null has no interpolation',
      input: null,
      expected: false,
    },
    {
      description: 'undefined has no interpolation',
      input: undefined,
      expected: false,
    },
    {
      description: 'boolean has no interpolation',
      input: true,
      expected: false,
    },
    {
      description: 'empty object has no interpolation',
      input: {},
      expected: false,
    },
    {
      description: 'empty array has no interpolation',
      input: [],
      expected: false,
    },
  ])('$description', ({ input, expected }) => {
    expect(hasInterpolationProperty(input)).toBe(expected);
  });
});
