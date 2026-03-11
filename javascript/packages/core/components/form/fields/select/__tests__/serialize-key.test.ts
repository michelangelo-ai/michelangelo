import { serializeKey } from '../serialize-key';

describe('serializeKey', () => {
  it('produces identical output regardless of key order', () => {
    expect(serializeKey({ a: 1, b: 2 })).toBe(serializeKey({ b: 2, a: 1 }));
  });

  it('handles nested objects with different key order', () => {
    const a = { outer: { z: 1, a: 2 }, name: 'test' };
    const b = { name: 'test', outer: { a: 2, z: 1 } };
    expect(serializeKey(a)).toBe(serializeKey(b));
  });

  it('handles deeply nested objects', () => {
    const a = { l1: { l2: { l3: { z: 'last', a: 'first' } } } };
    const b = { l1: { l2: { l3: { a: 'first', z: 'last' } } } };
    expect(serializeKey(a)).toBe(serializeKey(b));
  });

  it('preserves array order', () => {
    expect(serializeKey([3, 1, 2])).toBe('[3,1,2]');
    expect(serializeKey([3, 1, 2])).not.toBe(serializeKey([1, 2, 3]));
  });

  it('handles primitives', () => {
    expect(serializeKey('hello')).toBe('"hello"');
    expect(serializeKey(42)).toBe('42');
    expect(serializeKey(null)).toBe('null');
    expect(serializeKey(true)).toBe('true');
  });

  it('handles objects inside arrays', () => {
    const a = [{ b: 2, a: 1 }];
    const b = [{ a: 1, b: 2 }];
    expect(serializeKey(a)).toBe(serializeKey(b));
  });
});
