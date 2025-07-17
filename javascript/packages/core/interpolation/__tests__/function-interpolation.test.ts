import { FunctionInterpolation } from '../function-interpolation';

import type { InterpolationContext } from '../types';

describe('FunctionInterpolation', () => {
  test('executes function without parameters', () => {
    const fn = () => 'user is viewing';
    const interpolation = new FunctionInterpolation(fn);

    const result = interpolation.execute({});

    expect(result).toBe('user is viewing');
  });

  test('executes function with parameters', () => {
    const fn = ({ page }: InterpolationContext) => `user is viewing ${page.title}`;
    const interpolation = new FunctionInterpolation(fn);

    const result = interpolation.execute({ page: { title: 'Dashboard' } });

    expect(result).toBe('user is viewing Dashboard');
  });

  test('throws error for missing parameters', () => {
    const fn = ({ page }: InterpolationContext) => `user is viewing ${page.title}`;
    const interpolation = new FunctionInterpolation(fn);

    expect(() => interpolation.execute({})).toThrow(
      "Cannot read properties of undefined (reading 'title')"
    );
  });
});
