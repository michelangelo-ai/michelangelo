import { deleteByPath, getByPath } from '#core/components/form/utils/object-path';

describe('getByPath', () => {
  it('reads a top-level value', () => {
    expect(getByPath({ name: 'Alice' }, 'name')).toBe('Alice');
  });

  it('reads a nested value', () => {
    expect(getByPath({ spec: { a: { b: 'value' } } }, 'spec.a.b')).toBe('value');
  });

  it('reads an indexed value', () => {
    expect(getByPath({ items: [{ name: 'first' }, { name: 'second' }] }, 'items[1].name')).toBe(
      'second'
    );
  });

  it('returns undefined when a segment is missing', () => {
    expect(getByPath({ spec: {} }, 'spec.a.b')).toBeUndefined();
  });
});

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
