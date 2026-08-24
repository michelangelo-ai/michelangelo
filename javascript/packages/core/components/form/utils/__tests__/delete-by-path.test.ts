import { deleteByPath } from '#core/components/form/utils/delete-by-path';

describe('deleteByPath', () => {
  it('removes a top-level key', () => {
    expect(deleteByPath({ mode: 'basic', advancedSetting: 'stale' }, 'advancedSetting')).toEqual({
      mode: 'basic',
    });
  });

  it('removes a nested key without touching a sibling', () => {
    const values = { spec: { a: { b: 'stale', c: 'kept' } } };

    expect(deleteByPath(values, 'spec.a.b')).toEqual({ spec: { a: { c: 'kept' } } });
  });

  it('prunes a parent object left empty by the removal', () => {
    const values = { mode: 'basic', spec: { a: { b: 'stale' } } };

    expect(deleteByPath(values, 'spec.a.b')).toEqual({ mode: 'basic' });
  });

  it('compacts an array left with a hole by the removal', () => {
    const values = { items: [{ name: 'kept' }, { name: 'stale' }] };

    expect(deleteByPath(values, 'items[1].name')).toEqual({ items: [{ name: 'kept' }] });
  });

  it('does not mutate the original values', () => {
    const values = { spec: { a: { b: 'stale' } } };

    deleteByPath(values, 'spec.a.b');

    expect(values).toEqual({ spec: { a: { b: 'stale' } } });
  });
});
