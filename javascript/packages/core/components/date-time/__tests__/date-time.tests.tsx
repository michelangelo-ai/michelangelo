import { render, screen } from '@testing-library/react';

import { TimeZone } from '#core/types/time-types';
import { mockTimezone } from '#core/test/utils/mock-timezone';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getUserProviderWrapper } from '#core/test/wrappers/get-user-provider-wrapper';
import { DateTime } from '../date-time';

describe('DateTime', () => {
  const restore = mockTimezone();

  afterAll(() => {
    restore();
  });

  test('renders nothing when timestamp is undefined', () => {
    render(
      <DateTime timestamp={undefined} />,
      buildWrapper([getBaseProviderWrapper(), getUserProviderWrapper()])
    );

    expect(screen.queryByText(/2024/)).toBeNull();
  });

  test('renders formatted date in local timezone', () => {
    render(
      <DateTime timestamp="1720656639" />,
      buildWrapper([
        getBaseProviderWrapper(),
        getUserProviderWrapper({ timeZone: TimeZone.Local }),
      ])
    );

    expect(screen.getByText('2024/07/11 02:10:39 (GMT+2)')).toBeInTheDocument();
  });

  test('renders formatted date in UTC timezone', () => {
    render(
      <DateTime timestamp="1720656639" />,
      buildWrapper([
        getBaseProviderWrapper(),
        getUserProviderWrapper({ timeZone: TimeZone.UTC }),
      ])
    );

    expect(screen.getByText('2024/07/11 00:10:39 (UTC)')).toBeInTheDocument();
  });

  test('renders formatted date for numeric timestamp', () => {
    render(
      <DateTime timestamp={1720656639} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getUserProviderWrapper({ timeZone: TimeZone.Local }),
      ])
    );

    expect(screen.getByText('2024/07/11 02:10:39 (GMT+2)')).toBeInTheDocument();
  });
});
