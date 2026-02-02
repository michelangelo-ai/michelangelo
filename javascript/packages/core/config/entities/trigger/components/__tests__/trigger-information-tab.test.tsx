import { render, screen } from '@testing-library/react';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { TriggerInformationTab } from '../trigger-information-tab';

import type { TriggerRun } from '../../types';

const mockTriggerRun: TriggerRun = {
  metadata: {
    name: 'test-trigger',
    creationTimestamp: { seconds: 1704067200 },
  },
  spec: {
    actor: { name: 'test-user' },
    pipeline: { name: 'test-pipeline' },
    revision: { name: 'test-revision' },
    trigger: {
      triggerType: { case: 'cronSchedule', value: { cron: '0 0 * * *' } },
      parametersMap: {
        param1: { value: 'value1' },
        param2: { value: 'value2' },
      },
    },
  },
  status: {
    state: 4,
    logUrl: 'https://logs.example.com/trigger/123',
    errorMessage: 'Test error message',
  },
};

describe('TriggerInformationTab', () => {
  it('renders loading state', () => {
    render(
      <TriggerInformationTab data={undefined} isLoading={true} />,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.getByText('Loading...')).toBeInTheDocument();
  });

  it('renders no data state when data is undefined', () => {
    render(
      <TriggerInformationTab data={undefined} isLoading={false} />,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.getByText('No data available')).toBeInTheDocument();
  });

  it('renders log URL as clickable link', () => {
    render(
      <TriggerInformationTab data={mockTriggerRun} isLoading={false} />,
      buildWrapper([getBaseProviderWrapper()])
    );

    const logLink = screen.getByRole('link', { name: mockTriggerRun.status!.logUrl });
    expect(logLink).toHaveAttribute('href', mockTriggerRun.status!.logUrl);
    expect(logLink).toHaveAttribute('target', '_blank');
  });

  it('renders fallback message when log URL is empty', () => {
    const triggerRunWithoutLogUrl: TriggerRun = {
      ...mockTriggerRun,
      status: { state: 4 },
    };

    render(
      <TriggerInformationTab data={triggerRunWithoutLogUrl} isLoading={false} />,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.getByText('No log URL available')).toBeInTheDocument();
  });

  it('renders error message when present', () => {
    render(
      <TriggerInformationTab data={mockTriggerRun} isLoading={false} />,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.getByText('Error Message')).toBeInTheDocument();
    expect(screen.getByText('Test error message')).toBeInTheDocument();
  });

  it('does not render error section when error message is absent', () => {
    const triggerRunWithoutError: TriggerRun = {
      ...mockTriggerRun,
      status: { state: 4, logUrl: 'https://logs.example.com/trigger/123' },
    };

    render(
      <TriggerInformationTab data={triggerRunWithoutError} isLoading={false} />,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.queryByText('Error Message')).not.toBeInTheDocument();
  });

  it('renders parameters as JSON when present', () => {
    render(
      <TriggerInformationTab data={mockTriggerRun} isLoading={false} />,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.getByText('Parameters')).toBeInTheDocument();
    expect(screen.getByText(/"param1"/)).toBeInTheDocument();
  });

  it('does not render parameters section when params are empty', () => {
    const triggerRunWithoutParams: TriggerRun = {
      ...mockTriggerRun,
      spec: {
        ...mockTriggerRun.spec,
        trigger: {
          triggerType: { case: 'cronSchedule', value: { cron: '0 0 * * *' } },
        },
      },
    };

    render(
      <TriggerInformationTab data={triggerRunWithoutParams} isLoading={false} />,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.queryByText('Parameters')).not.toBeInTheDocument();
  });
});
