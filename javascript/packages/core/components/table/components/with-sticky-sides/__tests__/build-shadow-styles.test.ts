import { buildShadowStyles } from '../build-shadow-styles';

describe('buildShadowStyles', () => {
  it('positions right shadow correctly', () => {
    const result = buildShadowStyles('right', 0.5);

    expect(result['::before']).toMatchObject({
      right: '-7px',
      left: 'auto',
    });
  });

  it('positions left shadow correctly', () => {
    const result = buildShadowStyles('left', 0.3);

    expect(result['::before']).toMatchObject({
      right: 'auto',
      left: '-7px',
    });
  });

  it('handles no scroll state (opacity 0)', () => {
    const result = buildShadowStyles('right', -1);

    expect(result['::before']?.opacity).toBe(0);
  });

  it('calculates opacity based on scroll ratio', () => {
    expect(buildShadowStyles('right', 0)['::before']?.opacity).toBe(0);
    expect(buildShadowStyles('right', 1)['::before']?.opacity).toBe(1);
    expect(buildShadowStyles('left', 0)['::before']?.opacity).toBe(1);
    expect(buildShadowStyles('left', 1)['::before']?.opacity).toBe(0);
  });
});
