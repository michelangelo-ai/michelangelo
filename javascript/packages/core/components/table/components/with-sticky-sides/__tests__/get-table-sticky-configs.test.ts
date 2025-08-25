import { getTableStickyConfigs } from '../get-table-sticky-configs';

describe('getTableStickyConfigs', () => {
  it('generates config for selection + first data column', () => {
    const result = getTableStickyConfigs(true, 5);

    expect(result).toEqual({
      0: { stickySide: 'left', position: 0, shadowSide: 'none' },
      1: { stickySide: 'left', position: 56, shadowSide: 'right' },
      5: { stickySide: 'right', position: 0, shadowSide: 'left' },
    });
  });

  it('generates config for first data column only', () => {
    const result = getTableStickyConfigs(false, 3);

    expect(result).toEqual({
      1: { stickySide: 'left', position: 0, shadowSide: 'right' },
      3: { stickySide: 'right', position: 0, shadowSide: 'left' },
    });
  });
});
