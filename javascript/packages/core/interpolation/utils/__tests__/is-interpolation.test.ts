import { interpolate } from '#core/interpolation/interpolate';
import { isInterpolation } from '../is-interpolation';

describe('isInterpolation', () => {
  test.each([
    {
      description: 'string with interpolation pattern is interpolation',
      input: 'Hello ${user.name}',
      expected: true,
    },
    {
      description: 'string with only interpolation pattern is interpolation',
      input: '${page.title}',
      expected: true,
    },
    {
      description: 'plain string is not interpolation',
      input: 'Hello world',
      expected: false,
    },
    {
      description: 'empty string is not interpolation',
      input: '',
      expected: false,
    },
    {
      description: 'function interpolation instance is interpolation',
      input: interpolate(({ data }) => data.count as number),
      expected: true,
    },
    {
      description: 'number is not interpolation',
      input: 42,
      expected: false,
    },
    {
      description: 'null is not interpolation',
      input: null,
      expected: false,
    },
    {
      description: 'undefined is not interpolation',
      input: undefined,
      expected: false,
    },
    {
      description: 'object is not interpolation',
      input: {},
      expected: false,
    },
    {
      description: 'array is not interpolation',
      input: [],
      expected: false,
    },
    {
      description: 'boolean is not interpolation',
      input: true,
      expected: false,
    },
  ])('$description', ({ input, expected }) => {
    expect(isInterpolation(input)).toBe(expected);
  });
});
