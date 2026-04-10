import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

/* eslint-disable local/no-module-scope-test-setup -- restructure into nested describes, see https://github.com/michelangelo-ai/michelangelo/issues/1088 */

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

  const mockColumn = {
    id: 'department',
    label: 'Department',
    type: 'text',
  };

  const defaultProps: ColumnFilterProps = {
    column: mockColumn,
    close: mockClose,
    getFilterValue: mockGetFilterValue,
    setFilterValue: mockSetFilterValue,
    preFilteredRows: [
      { getValue: () => 'Engineering', record: { department: 'Engineering' } },
      { getValue: () => 'Marketing', record: { department: 'Marketing' } },
      { getValue: () => 'Engineering', record: { department: 'Engineering' } },
      { getValue: () => 'Sales', record: { department: 'Sales' } },
      { getValue: () => 'Design', record: { department: 'Design' } },
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

    it('should select all values when "Select all" is clicked', async () => {
      const user = userEvent.setup();
      mockGetFilterValue.mockReturnValue(['Engineering']);

      render(
        <CategoricalFilter {...defaultProps} />,
        buildWrapper([getBaseProviderWrapper(), getInterpolationProviderWrapper()])
      );

      await user.click(screen.getByRole('checkbox', { name: 'Select All' }));
      await user.click(screen.getByRole('button', { name: 'Apply' }));

      expect(mockSetFilterValue).toHaveBeenCalledWith(
        expect.arrayContaining(['Design', 'Engineering', 'Marketing', 'Sales'])
      );
      expect(mockClose).toHaveBeenCalled();
    });

    it('should clear all values when "Clear" is clicked', async () => {
      const user = userEvent.setup();
      mockGetFilterValue.mockReturnValue(['Engineering', 'Sales']);

      render(
        <CategoricalFilter {...defaultProps} />,
        buildWrapper([getBaseProviderWrapper(), getInterpolationProviderWrapper()])
      );

      await user.click(screen.getByRole('checkbox', { name: 'Clear' }));
      await user.click(screen.getByRole('button', { name: 'Apply' }));

      expect(mockSetFilterValue).toHaveBeenCalledWith(undefined);
      expect(mockClose).toHaveBeenCalled();
    });
  });

  describe('data extraction', () => {
    it('should extract unique values from preFilteredRows', () => {
      const propsWithDuplicates: ColumnFilterProps = {
        ...defaultProps,
        preFilteredRows: [
          { getValue: () => 'Engineering', record: { department: 'Engineering' } },
          { getValue: () => 'Engineering', record: { department: 'Engineering' } },
          { getValue: () => 'Marketing', record: { department: 'Marketing' } },
          { getValue: () => 'Engineering', record: { department: 'Engineering' } },
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
          { getValue: () => 'Engineering', record: { department: 'Engineering' } },
          { getValue: () => null, record: { department: null } },
          { getValue: () => undefined, record: { department: undefined } },
          { getValue: () => 'Marketing', record: { department: 'Marketing' } },
          { getValue: () => '', record: { department: '' } },
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

  it('should show display text but filter with raw values when cellToString differs from raw data', async () => {
    const user = userEvent.setup();

    const stateColumn = {
      id: 'status',
      label: 'Status',
      type: 'STATE',
      stateTextMap: {
        PIPELINE_STATE_BUILDING: 'Building',
        PIPELINE_STATE_ERROR: 'Failed',
        PIPELINE_STATE_SUCCESS: 'Complete',
      },
    };

    const stateProps: ColumnFilterProps = {
      column: stateColumn,
      close: mockClose,
      getFilterValue: mockGetFilterValue,
      setFilterValue: mockSetFilterValue,
      preFilteredRows: [
        {
          getValue: () => 'PIPELINE_STATE_BUILDING',
          record: { status: 'PIPELINE_STATE_BUILDING' },
        },
        { getValue: () => 'PIPELINE_STATE_ERROR', record: { status: 'PIPELINE_STATE_ERROR' } },
        { getValue: () => 'PIPELINE_STATE_SUCCESS', record: { status: 'PIPELINE_STATE_SUCCESS' } },
      ],
    };

    render(
      <CategoricalFilter {...stateProps} />,
      buildWrapper([getBaseProviderWrapper(), getInterpolationProviderWrapper()])
    );

    // Should show display text in filter options
    expect(screen.getByLabelText('Building')).toBeInTheDocument();
    expect(screen.getByLabelText('Failed')).toBeInTheDocument();
    expect(screen.getByLabelText('Complete')).toBeInTheDocument();

    // Select display value and apply filter
    await user.click(screen.getByLabelText('Building'));
    await user.click(screen.getByRole('button', { name: 'Apply' }));

    // Should filter with raw value, not display text
    expect(mockSetFilterValue).toHaveBeenCalledWith(['PIPELINE_STATE_BUILDING']);
    expect(mockClose).toHaveBeenCalled();
  });
});
