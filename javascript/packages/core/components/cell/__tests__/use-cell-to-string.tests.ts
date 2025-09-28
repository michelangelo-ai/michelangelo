import { renderHook } from '@testing-library/react';

import { useCellToString } from '../use-cell-to-string';

describe('useCellToString', () => {
  it('should return undefined for null value', () => {
    const { result } = renderHook(() => useCellToString());
    const cellToString = result.current;

    const props = {
      value: null,
      column: { type: 'text', id: 'test' },
      record: {},
    };
    expect(cellToString(props)).toBeUndefined();
  });

  it('should return undefined for undefined value', () => {
    const { result } = renderHook(() => useCellToString());
    const cellToString = result.current;

    const props = {
      value: undefined,
      column: { type: 'text', id: 'test' },
      record: {},
    };
    expect(cellToString(props)).toBeUndefined();
  });

  it('should return undefined for empty string value', () => {
    const { result } = renderHook(() => useCellToString());
    const cellToString = result.current;

    const props = {
      value: '',
      column: { type: 'text', id: 'test' },
      record: {},
    };
    expect(cellToString(props)).toBeUndefined();
  });

  it('should return string for string value', () => {
    const { result } = renderHook(() => useCellToString());
    const cellToString = result.current;

    const props = {
      value: 'test',
      column: { type: 'unknown', id: 'test' },
      record: {},
    };
    expect(cellToString(props)).toBe('test');
  });

  it('should convert number to string when no renderer is available', () => {
    const { result } = renderHook(() => useCellToString());
    const cellToString = result.current;

    const props = {
      value: 123,
      column: { type: 'unknown', id: 'test' },
      record: {},
    };
    expect(cellToString(props)).toBe('123');
  });

  it('should convert boolean to string when no renderer is available', () => {
    const { result } = renderHook(() => useCellToString());
    const cellToString = result.current;

    const props = {
      value: true,
      column: { type: 'unknown', id: 'test' },
      record: {},
    };
    expect(cellToString(props)).toBe('true');
  });

  it('should handle complex objects', () => {
    const { result } = renderHook(() => useCellToString());
    const cellToString = result.current;

    const props = {
      value: { foo: 'bar' },
      column: { type: 'text', id: 'test' },
      record: {},
    };
    expect(cellToString(props)).toEqual('{"foo":"bar"}');
  });

  it('should handle custom renderer', () => {
    const { result } = renderHook(() => useCellToString());
    const cellToString = result.current;

    const props = {
      value: true,
      column: { type: 'boolean', id: 'test' },
      record: { test: true },
    };
    expect(cellToString(props)).toBe('true');
  });
});
