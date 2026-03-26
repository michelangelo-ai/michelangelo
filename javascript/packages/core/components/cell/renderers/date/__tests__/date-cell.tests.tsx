import { render, screen } from '@testing-library/react';

import { mockTimezone } from '#core/test/utils/mock-timezone';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getUserProviderWrapper } from '#core/test/wrappers/get-user-provider-wrapper';
import { TimeZone } from '#core/types/time-types';
import { DateCell } from '../date-cell';

describe('DateCell', () => {
  const restore = mockTimezone();

  afterAll(() => {
    restore();
  });

  test('Renders nothing for empty value', () => {
    render(
      <DateCell
        column={{ id: 'spec.date' }}
        record={{ spec: { date: undefined } }}
        value={undefined}
      />,
      buildWrapper([getBaseProviderWrapper(), getUserProviderWrapper()])
    );

    expect(screen.queryByText(/2024/)).toBeNull();
  });

  test('Renders formatted date for valid timestamp in local timezone', () => {
    render(
      <DateCell
        column={{ id: 'spec.date' }}
        record={{ spec: { date: '1720656639' } }}
        value="1720656639"
      />,
      buildWrapper([getBaseProviderWrapper(), getUserProviderWrapper({ timeZone: TimeZone.Local })])
    );

    expect(screen.getByText('2024/07/11 02:10:39 (GMT+2)')).toBeInTheDocument();
  });

  test('Renders UTC date when timezone is UTC', () => {
    render(
      <DateCell
        column={{ id: 'spec.date' }}
        record={{ spec: { date: '1720656639' } }}
        value="1720656639"
      />,
      buildWrapper([getBaseProviderWrapper(), getUserProviderWrapper()])
    );

    expect(screen.getByText('2024/07/11 00:10:39 (UTC)')).toBeInTheDocument();
  });

  describe('toString', () => {
    test('Returns empty string for empty value', () => {
      expect(DateCell.toString({ value: undefined, column: { id: 'spec.date' } })).toBe('');
    });

    test('Returns formatted date string', () => {
      expect(
        DateCell.toString({
          value: '1720656639',
          column: { id: 'spec.date' },
        })
      ).toBe('2024/07/11 02:10:39 (GMT+2)');
    });
  });
});
