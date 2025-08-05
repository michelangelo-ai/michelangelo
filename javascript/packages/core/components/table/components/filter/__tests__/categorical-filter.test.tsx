import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getInterpolationProviderWrapper } from '#core/test/wrappers/get-interpolation-provider-wrapper';
import { CategoricalFilter } from '../categorical/categorical-filter';

import type { ColumnFilterProps } from '../types';

/**
 * BaseUI CategoricalColumn.Filter has 3 checkboxes: **Select all**, **Clear**, **Exclude**
 */
const BASE_UI_CHECKBOX_COUNT = 3;

describe('CategoricalFilter', () => {
  const mockClose = vi.fn();
  const mockSetFilterValue = vi.fn();
  const mockGetFilterValue = vi.fn();

  const defaultProps: ColumnFilterProps = {
    columnId: 'department',
    close: mockClose,
    getFilterValue: mockGetFilterValue,
    setFilterValue: mockSetFilterValue,
    preFilteredRows: [
      { getValue: () => 'Engineering' },
      { getValue: () => 'Marketing' },
      { getValue: () => 'Engineering' },
      { getValue: () => 'Sales' },
      { getValue: () => 'Design' },
    ],
  };

  beforeEach(() => {
    vi.clearAllMocks();
    mockGetFilterValue.mockReturnValue(undefined);
  });

  describe('sorting logic', () => {
    it('should sort values alphabetically when no filters are selected', () => {
      render(
        <CategoricalFilter {...defaultProps} />,
        buildWrapper([getBaseProviderWrapper(), getInterpolationProviderWrapper()])
      );

      // Check that all expected values are present
      expect(screen.getByLabelText('Design')).toBeInTheDocument();
      expect(screen.getByLabelText('Engineering')).toBeInTheDocument();
      expect(screen.getByLabelText('Marketing')).toBeInTheDocument();
      expect(screen.getByLabelText('Sales')).toBeInTheDocument();
    });

    it('should sort selected values first, then alphabetical', () => {
      mockGetFilterValue.mockReturnValue(['Sales', 'Design']);

      render(
        <CategoricalFilter {...defaultProps} />,
        buildWrapper([getBaseProviderWrapper(), getInterpolationProviderWrapper()])
      );

      expect(screen.getByLabelText('Design')).toBeChecked();
      expect(screen.getByLabelText('Sales')).toBeChecked();
      expect(screen.getByLabelText('Engineering')).not.toBeChecked();
      expect(screen.getByLabelText('Marketing')).not.toBeChecked();
    });
  });

  it('should show no checkboxes checked when no filter is applied', () => {
    render(
      <CategoricalFilter {...defaultProps} />,
      buildWrapper([getBaseProviderWrapper(), getInterpolationProviderWrapper()])
    );

    expect(screen.getByLabelText('Design')).not.toBeChecked();
    expect(screen.getByLabelText('Engineering')).not.toBeChecked();
    expect(screen.getByLabelText('Marketing')).not.toBeChecked();
    expect(screen.getByLabelText('Sales')).not.toBeChecked();
  });

  describe('user interactions', () => {
    it('should call setFilterValue and close when applying selection', async () => {
      const user = userEvent.setup();

      render(
        <CategoricalFilter {...defaultProps} />,
        buildWrapper([getBaseProviderWrapper(), getInterpolationProviderWrapper()])
      );

      await user.click(screen.getByLabelText('Engineering'));
      await user.click(screen.getByRole('button', { name: 'Apply' }));

      expect(mockSetFilterValue).toHaveBeenCalledWith(['Engineering']);
      expect(mockClose).toHaveBeenCalled();
    });

    it('should set undefined when no values are selected', async () => {
      const user = userEvent.setup();
      mockGetFilterValue.mockReturnValue(['Engineering']);

      render(
        <CategoricalFilter {...defaultProps} />,
        buildWrapper([getBaseProviderWrapper(), getInterpolationProviderWrapper()])
      );

      // Uncheck the only selected item
      await user.click(screen.getByLabelText('Engineering'));
      await user.click(screen.getByRole('button', { name: 'Apply' }));

      expect(mockSetFilterValue).toHaveBeenCalledWith(undefined);
      expect(mockClose).toHaveBeenCalled();
    });

    it('should handle multiple selections', async () => {
      const user = userEvent.setup();

      render(
        <CategoricalFilter {...defaultProps} />,
        buildWrapper([getBaseProviderWrapper(), getInterpolationProviderWrapper()])
      );

      await user.click(screen.getByLabelText('Engineering'));
      await user.click(screen.getByLabelText('Design'));
      await user.click(screen.getByRole('button', { name: 'Apply' }));

      expect(mockSetFilterValue).toHaveBeenCalledWith(
        expect.arrayContaining(['Design', 'Engineering'])
      );
      expect(mockClose).toHaveBeenCalled();
    });

    it('should handle exclude logic when exclude is checked', async () => {
      const user = userEvent.setup();

      render(
        <CategoricalFilter {...defaultProps} />,
        buildWrapper([getBaseProviderWrapper(), getInterpolationProviderWrapper()])
      );

      // Select Engineering and Design
      await user.click(screen.getByLabelText('Engineering'));
      await user.click(screen.getByLabelText('Design'));

      // Enable exclude mode
      await user.click(screen.getByLabelText('Exclude'));
      await user.click(screen.getByRole('button', { name: 'Apply' }));

      // Should filter to everything EXCEPT Engineering and Design (i.e., Marketing and Sales)
      expect(mockSetFilterValue).toHaveBeenCalledWith(
        expect.arrayContaining(['Marketing', 'Sales'])
      );
      expect(mockClose).toHaveBeenCalled();
    });

    it('should return undefined when exclude mode results in empty selection', async () => {
      const user = userEvent.setup();

      render(
        <CategoricalFilter {...defaultProps} />,
        buildWrapper([getBaseProviderWrapper(), getInterpolationProviderWrapper()])
      );

      // Select all values
      await user.click(screen.getByLabelText('Engineering'));
      await user.click(screen.getByLabelText('Design'));
      await user.click(screen.getByLabelText('Marketing'));
      await user.click(screen.getByLabelText('Sales'));

      // Enable exclude mode (should exclude everything)
      await user.click(screen.getByLabelText('Exclude'));
      await user.click(screen.getByRole('button', { name: 'Apply' }));

      // Should clear the filter since excluding all values leaves nothing
      expect(mockSetFilterValue).toHaveBeenCalledWith(undefined);
      expect(mockClose).toHaveBeenCalled();
    });
  });

  describe('data extraction', () => {
    it('should extract unique values from preFilteredRows', () => {
      const propsWithDuplicates: ColumnFilterProps = {
        ...defaultProps,
        preFilteredRows: [
          { getValue: () => 'Engineering' },
          { getValue: () => 'Engineering' },
          { getValue: () => 'Marketing' },
          { getValue: () => 'Engineering' },
        ],
      };

      render(
        <CategoricalFilter {...propsWithDuplicates} />,
        buildWrapper([getBaseProviderWrapper(), getInterpolationProviderWrapper()])
      );

      // getByLabelText will fail if there are multiple matches
      expect(screen.getByLabelText('Engineering')).toBeInTheDocument();
      expect(screen.getByLabelText('Marketing')).toBeInTheDocument();

      expect(screen.getAllByRole('checkbox')).toHaveLength(BASE_UI_CHECKBOX_COUNT + 2);
    });

    it('should handle null and undefined values gracefully', () => {
      const propsWithNulls: ColumnFilterProps = {
        ...defaultProps,
        preFilteredRows: [
          { getValue: () => 'Engineering' },
          { getValue: () => null },
          { getValue: () => undefined },
          { getValue: () => 'Marketing' },
          { getValue: () => '' },
        ],
      };

      render(
        <CategoricalFilter {...propsWithNulls} />,
        buildWrapper([getBaseProviderWrapper(), getInterpolationProviderWrapper()])
      );

      // Should only show non-null values (empty string is still a valid value)
      expect(screen.getByLabelText('Engineering')).toBeInTheDocument();
      expect(screen.getByLabelText('Marketing')).toBeInTheDocument();
      expect(screen.getByLabelText('')).toBeInTheDocument();

      expect(screen.getAllByRole('checkbox')).toHaveLength(BASE_UI_CHECKBOX_COUNT + 3);
    });
  });
});
