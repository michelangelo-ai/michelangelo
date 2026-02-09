import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { RetryCell } from '#core/components/cell/renderers/retry/retry-cell';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import {
  createQueryMockRouter,
  getServiceProviderWrapper,
} from '#core/test/wrappers/get-service-provider-wrapper';

import type { ServiceContextType } from '#core/providers/service-provider/types';

describe('RetryCell', () => {
  const mockPipelineRunData = {
    pipelineRun: {
      spec: { existingKey: 'existingValue' },
      status: { state: 4, workflowId: 'wf-123', workflowRunId: 'wfr-456' },
    },
  };

  const defaultProps = {
    column: { id: 'retry' },
    record: {},
    value: 'activity-123',
  };

  function renderRetryCell(
    mockRequest: ServiceContextType['request'],
    props: Partial<typeof defaultProps> = {}
  ) {
    return render(
      <RetryCell {...defaultProps} {...props} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/test-project/train/runs/test-run' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );
  }

  it('renders nothing when value is empty', () => {
    const mockRequest = createQueryMockRouter({
      GetPipelineRun: mockPipelineRunData,
    });

    renderRetryCell(mockRequest, { value: '' });

    expect(screen.queryByRole('button', { name: 'Retry' })).not.toBeInTheDocument();
  });

  it('renders nothing when pipeline run is not terminated', async () => {
    const runningData = {
      pipelineRun: {
        spec: { existingKey: 'existingValue' },
        status: { state: 1, workflowId: 'wf-123', workflowRunId: 'wfr-456' },
      },
    };

    const mockRequest = createQueryMockRouter({
      GetPipelineRun: runningData,
    });

    renderRetryCell(mockRequest);

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        'GetPipelineRun',
        expect.objectContaining({ namespace: 'test-project', name: 'test-run' })
      );
    });

    expect(screen.queryByRole('button', { name: 'Retry' })).not.toBeInTheDocument();
  });

  it('renders retry button for terminated pipeline run', async () => {
    const mockRequest = createQueryMockRouter({
      GetPipelineRun: mockPipelineRunData,
    });

    renderRetryCell(mockRequest);

    expect(await screen.findByRole('button', { name: 'Retry' })).toBeInTheDocument();
  });

  it('opens dialog on retry button click', async () => {
    const user = userEvent.setup();
    const mockRequest = createQueryMockRouter({
      GetPipelineRun: mockPipelineRunData,
    });

    renderRetryCell(mockRequest);

    await user.click(await screen.findByRole('button', { name: 'Retry' }));

    const dialog = await screen.findByRole('dialog', { name: 'Retry Task' });
    expect(within(dialog).getByText('Are you sure you want to retry this task?')).toBeInTheDocument();
    expect(within(dialog).getByRole('textbox')).toBeInTheDocument();
  });

  it('submits retry with correct data and closes dialog', async () => {
    const user = userEvent.setup();
    const mockRequest = createQueryMockRouter({
      GetPipelineRun: mockPipelineRunData,
      UpdatePipelineRun: { pipelineRun: {} },
    });

    renderRetryCell(mockRequest);

    await user.click(await screen.findByRole('button', { name: 'Retry' }));
    const dialog = await screen.findByRole('dialog', { name: 'Retry Task' });
    await user.click(within(dialog).getByRole('button', { name: 'Retry Task' }));

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith('UpdatePipelineRun', {
        pipelineRun: expect.objectContaining({
          spec: expect.objectContaining({
            existingKey: 'existingValue',
            retryInfo: {
              activityId: 'activity-123',
              workflowId: 'wf-123',
              workflowRunId: 'wfr-456',
              reason: 'Manual retry from UI',
            },
          }) as Record<string, unknown>,
        }) as Record<string, unknown>,
      });
    });

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
  });

  it('keeps dialog open and displays error on failure', async () => {
    const user = userEvent.setup();
    const mockRequest = createQueryMockRouter({
      GetPipelineRun: mockPipelineRunData,
      UpdatePipelineRun: new Error('Test error'),
    });

    renderRetryCell(mockRequest);

    await user.click(await screen.findByRole('button', { name: 'Retry' }));
    const dialog = await screen.findByRole('dialog', { name: 'Retry Task' });
    await user.click(within(dialog).getByRole('button', { name: 'Retry Task' }));

    await screen.findByText(/Test error/);
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('submits with custom retry reason', async () => {
    const user = userEvent.setup();
    const mockRequest = createQueryMockRouter({
      GetPipelineRun: mockPipelineRunData,
      UpdatePipelineRun: { pipelineRun: {} },
    });

    renderRetryCell(mockRequest);

    await user.click(await screen.findByRole('button', { name: 'Retry' }));
    const dialog = await screen.findByRole('dialog', { name: 'Retry Task' });

    const textarea = within(dialog).getByRole('textbox');
    await user.clear(textarea);
    await user.type(textarea, 'Pipeline failed due to OOM');

    await user.click(within(dialog).getByRole('button', { name: 'Retry Task' }));

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        'UpdatePipelineRun',
        expect.objectContaining({
          pipelineRun: expect.objectContaining({
            spec: expect.objectContaining({
              retryInfo: expect.objectContaining({
                reason: 'Pipeline failed due to OOM',
              }),
            }),
          }) as Record<string, unknown>,
        })
      );
    });
  });

  it('closes dialog on cancel', async () => {
    const user = userEvent.setup();
    const mockRequest = createQueryMockRouter({
      GetPipelineRun: mockPipelineRunData,
    });

    renderRetryCell(mockRequest);

    await user.click(await screen.findByRole('button', { name: 'Retry' }));
    await screen.findByRole('dialog', { name: 'Retry Task' });

    await user.click(screen.getByRole('button', { name: 'Cancel' }));

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
  });
});
