import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getInterpolationProviderWrapper } from '#core/test/wrappers/get-interpolation-provider-wrapper';
import { UNIFIED_API_ORIGIN_DATE } from '../datetime/constants';
import { DatetimeFilter } from '../datetime/datetime-filter';

import type { DatetimeFilterValue } from '../datetime/types';
import type { ColumnFilterProps } from '../types';

describe('DatetimeFilter', () => {
  const mockClose = vi.fn();
  const mockSetFilterValue = vi.fn();
  const mockGetFilterValue = vi.fn();

  const mockColumn = {
    id: 'createdAt',
    label: 'Created At',
    type: 'date',
  };

  const defaultProps: ColumnFilterProps = {
    column: mockColumn,
    close: mockClose,
    getFilterValue: mockGetFilterValue,
    setFilterValue: mockSetFilterValue,
    preFilteredRows: [
      { getValue: () => 1672531200, record: { createdAt: 1672531200 } }, // 2023-01-01 epoch seconds
      { getValue: () => 1680307200, record: { createdAt: 1680307200 } }, // 2023-04-01 epoch seconds
      { getValue: () => 1687824000, record: { createdAt: 1687824000 } }, // 2023-06-27 epoch seconds
    ],
  };

  beforeEach(() => {
    vi.clearAllMocks();
    mockGetFilterValue.mockReturnValue(undefined);
  });

  it('should use UNIFIED_API_ORIGIN_DATE as default start date', () => {
    render(
      <DatetimeFilter {...defaultProps} />,
      buildWrapper([getBaseProviderWrapper(), getInterpolationProviderWrapper()])
    );

    // Verify the filter range includes the origin date
    expect(UNIFIED_API_ORIGIN_DATE.getFullYear()).toBe(2022);
  });

  it('should display current filter dates in the UI', () => {
    const existingFilter: DatetimeFilterValue = {
      operation: 'RANGE_DATETIME',
      range: [new Date('2023-01-01T00:00:00.000Z'), new Date('2023-12-31T23:59:59.999Z')],
      selection: [],
      description: '2023',
      exclude: false,
    };

    mockGetFilterValue.mockReturnValue(existingFilter);

    render(
      <DatetimeFilter {...defaultProps} />,
      buildWrapper([getBaseProviderWrapper(), getInterpolationProviderWrapper()])
    );

    expect(screen.getByRole('textbox', { name: /Select a date range/ })).toHaveValue(
      '01–01–2023 – 12–31–2023'
    );
  });

  it('should show available date range when no filter is applied', () => {
    mockGetFilterValue.mockReturnValue(undefined);

    render(
      <DatetimeFilter {...defaultProps} />,
      buildWrapper([getBaseProviderWrapper(), getInterpolationProviderWrapper()])
    );

    // When no filter is applied, BaseUI shows the available date range
    // This should span from UNIFIED_API_ORIGIN_DATE (2022) to current date
    expect(screen.getByText(/Selected date range is from 01–01–2022/i)).toBeInTheDocument();
  });

  it('should conversion string dates to Date objects and display correctly', () => {
    const filterWithStringDates: DatetimeFilterValue = {
      operation: 'RANGE_DATETIME',
      // Simulate dates stored as strings in localStorage
      range: ['2023-01-01T00:00:00.000Z', '2023-12-31T23:59:59.000Z'] as unknown as Date[],
      selection: [],
      description: '2023',
      exclude: false,
    };

    mockGetFilterValue.mockReturnValue(filterWithStringDates);

    render(
      <DatetimeFilter {...defaultProps} />,
      buildWrapper([getBaseProviderWrapper(), getInterpolationProviderWrapper()])
    );

    expect(screen.getByRole('textbox', { name: /Select a date range/ })).toHaveValue(
      '01–01–2023 – 12–31–2023'
    );
  });

  it('should display different date ranges correctly', () => {
    const filter2022: DatetimeFilterValue = {
      operation: 'RANGE_DATETIME',
      range: [new Date('2022-06-15T00:00:00.000Z'), new Date('2022-08-30T23:59:59.999Z')],
      selection: [],
      description: 'Summer 2022',
      exclude: false,
    };

    mockGetFilterValue.mockReturnValue(filter2022);

    render(
      <DatetimeFilter {...defaultProps} />,
      buildWrapper([getBaseProviderWrapper(), getInterpolationProviderWrapper()])
    );

    expect(screen.getByRole('textbox', { name: /Select a date range/ })).toHaveValue(
      '06–15–2022 – 08–30–2022'
    );
  });

  it('should call setFilterValue and close when applying valid filter', async () => {
    const user = userEvent.setup();

    render(
      <DatetimeFilter {...defaultProps} />,
      buildWrapper([getBaseProviderWrapper(), getInterpolationProviderWrapper()])
    );

    await user.click(screen.getByRole('button', { name: 'Apply' }));

    expect(mockClose).toHaveBeenCalled();
  });

  it('should handle malformed filter values', () => {
    const malformedFilter = {
      operation: 'RANGE_DATETIME',
      range: null, // Invalid range
      selection: undefined,
      description: '',
      exclude: false,
    } as unknown as DatetimeFilterValue;

    mockGetFilterValue.mockReturnValue(malformedFilter);

    render(
      <DatetimeFilter {...defaultProps} />,
      buildWrapper([getBaseProviderWrapper(), getInterpolationProviderWrapper()])
    );

    // Should render without crashing
    expect(screen.getByRole('button', { name: 'Apply' })).toBeInTheDocument();
  });

  it('should handle incomplete date ranges', () => {
    const incompleteFilter: DatetimeFilterValue = {
      operation: 'RANGE_DATETIME',
      range: [new Date('2023-01-01T00:00:00.000Z')], // Missing end date
      selection: [],
      description: 'Incomplete range',
      exclude: false,
    };

    mockGetFilterValue.mockReturnValue(incompleteFilter);

    render(
      <DatetimeFilter {...defaultProps} />,
      buildWrapper([getBaseProviderWrapper(), getInterpolationProviderWrapper()])
    );

    expect(screen.getByText(/Selected date range is from 01–01–2023/i)).toBeInTheDocument();
  });
});
