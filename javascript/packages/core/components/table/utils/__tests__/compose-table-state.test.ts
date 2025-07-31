import { vi } from 'vitest';

import { composeTableState } from '../compose-table-state';

describe('composeTableState', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Suppress console.warn for tests that intentionally trigger warnings
    vi.spyOn(console, 'warn').mockImplementation(() => null);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe('controlled vs uncontrolled state detection', () => {
    it('routes to controlled state when both value and setter are provided', () => {
      const mockSetter = vi.fn();
      const input = {
        globalFilter: 'test-filter',
        setGlobalFilter: mockSetter,
      };

      const result = composeTableState(input);

      expect(result.state).toEqual({
        globalFilter: 'test-filter',
        setGlobalFilter: mockSetter,
      });
      expect(result.initialState).toEqual({});
    });

    it('routes to uncontrolled state when only value is provided', () => {
      const input = {
        globalFilter: 'initial-filter',
      };

      const result = composeTableState(input);

      expect(result.state).toEqual({});
      expect(result.initialState).toEqual({
        globalFilter: 'initial-filter',
      });
    });

    it('handles empty state object', () => {
      const result = composeTableState({});

      expect(result.state).toEqual({});
      expect(result.initialState).toEqual({});
    });
  });

  describe('validation and warnings', () => {
    it('warns when setter exists without corresponding value', () => {
      const mockSetter = vi.fn();
      const input = {
        setGlobalFilter: mockSetter,
        // Missing globalFilter value
      };

      composeTableState(input);

      expect(console.warn).toHaveBeenCalledWith(
        'Controlled state setter setGlobalFilter must be accompanied by property globalFilter'
      );
    });

    it('does not warn for valid controlled state', () => {
      const input = {
        globalFilter: 'test',
        setGlobalFilter: vi.fn(),
      };

      composeTableState(input);

      expect(console.warn).not.toHaveBeenCalled();
    });

    it('does not warn for valid uncontrolled state', () => {
      const input = {
        globalFilter: 'test',
      };

      composeTableState(input);

      expect(console.warn).not.toHaveBeenCalled();
    });
  });

  describe('edge cases', () => {
    it('handles setter without value by creating controlled state with undefined value', () => {
      const mockSetter = vi.fn();
      const input = {
        setGlobalFilter: mockSetter,
      };

      const result = composeTableState(input);

      expect(result.state).toEqual({
        globalFilter: undefined,
        setGlobalFilter: mockSetter,
      });
      expect(result.initialState).toEqual({});
    });
  });
});
