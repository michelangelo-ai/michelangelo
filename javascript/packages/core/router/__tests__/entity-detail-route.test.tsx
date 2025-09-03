import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import { CellType } from '#core/components/cell/constants';
import { CustomDetailPageConfig } from '#core/components/views/detail-view/types/detail-view-schema-types';
import { buildExecutionSchemaFactory } from '#core/components/views/execution/__fixtures__/execution-schema-factory';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { getServiceProviderWrapper } from '#core/test/wrappers/get-service-provider-wrapper';
import {
  buildEntityConfigFactory,
  buildPhaseConfigFactory,
} from '../__fixtures__/phase-config-factory';
import { EntityDetailRoute } from '../entity-detail-route';

describe('EntityDetailRoute', () => {
  const buildEntity = buildEntityConfigFactory({
    id: 'runs',
    name: 'Pipeline Runs',
    service: 'pipelineRun',
  });
  const buildExecutionSchema = buildExecutionSchemaFactory();
  const buildPhase = buildPhaseConfigFactory();

  test('renders execution tab', async () => {
    const testPhases = {
      train: buildPhase({
        id: 'train',
        entities: [
          buildEntity({
            views: [
              {
                type: 'detail',
                metadata: [
                  {
                    id: 'metadata.creationTimestamp.seconds',
                    label: 'Created',
                    type: CellType.DATE,
                  },
                  { id: 'status.state', label: 'State', type: CellType.STATE },
                ],
                pages: [
                  {
                    id: 'execution',
                    label: 'Execution',
                    ...buildExecutionSchema(),
                  },
                ],
              },
            ],
          }),
        ],
      }),
    };

    const mockEntityData = {
      pipelineRun: {
        metadata: {
          creationTimestamp: {
            seconds: 1640995200, // 2022-01-01
          },
        },
        status: {
          state: 'RUNNING',
          steps: [
            {
              displayName: 'Data Preparation',
              state: 'SUCCEEDED',
              subSteps: [],
            },
            {
              displayName: 'Model Training',
              state: 'RUNNING',
              subSteps: [],
            },
          ],
        },
      },
    };
    const mockRequest = vi.fn().mockResolvedValue(mockEntityData);

    render(
      <EntityDetailRoute phases={testPhases} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({
          location: '/myproject/train/runs/run-123',
        }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    expect(screen.getByRole('button', { name: /go back/i })).toBeInTheDocument();
    expect(screen.getByText('Pipeline Runs')).toBeInTheDocument(); // subtitle from entity config
    expect(screen.getByText('run-123')).toBeInTheDocument(); // title from URL entityId

    // Wait for and verify metadata is rendered
    expect(await screen.findByText('State')).toBeInTheDocument();
    expect(await screen.findByText('Running')).toBeInTheDocument();

    // Verify minimal execution tab functionality
    expect(screen.getByText('Execution')).toBeInTheDocument();
    await screen.findAllByText('Data Preparation');
    await screen.findAllByText('Model Training');
  });

  test('renders custom detail pages and navigates between them', async () => {
    const user = userEvent.setup();

    const testPhases = {
      train: buildPhase({
        id: 'train',
        entities: [
          buildEntity({
            views: [
              {
                type: 'detail',
                metadata: [{ id: 'status.state', label: 'State', type: CellType.STATE }],
                pages: [
                  {
                    id: 'first-page',
                    label: 'First page',
                    type: 'custom',
                    component: () => <div>First page component</div>,
                  } as CustomDetailPageConfig,
                  {
                    id: 'second-page',
                    label: 'Second page',
                    type: 'custom',
                    component: () => <div>Second page component</div>,
                  } as CustomDetailPageConfig,
                ],
              },
            ],
          }),
        ],
      }),
    };

    const mockRequest = vi.fn().mockResolvedValue({
      pipelineRun: {
        metadata: {
          creationTimestamp: {
            seconds: 1640995200, // 2022-01-01
          },
        },
        status: {
          state: 'SUCCESS',
        },
      },
    });

    render(
      <EntityDetailRoute phases={testPhases} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/train/runs/run-123' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    await screen.findByText('First page component');
    await user.click(await screen.findByText('Second page'));
    await screen.findByText('Second page component');
    expect(
      screen.getByText('Current pathname: /myproject/train/runs/run-123/second-page')
    ).toBeInTheDocument();
  });

  test('handles unknown page types', () => {
    const testPhases = {
      train: buildPhase({
        id: 'train',
        entities: [
          buildEntity({
            views: [
              {
                type: 'detail',
                metadata: [{ id: 'status.state', label: 'State', type: CellType.STATE }],
                pages: [
                  { id: 'unknown-type', label: 'Unknown Type', type: 'some-unknown-type' },
                  {
                    id: 'execution',
                    label: 'Execution',
                    ...buildExecutionSchema(),
                  },
                ],
              },
            ],
          }),
        ],
      }),
    };

    const mockRequest = vi.fn().mockResolvedValue({
      pipelineRun: {
        metadata: {
          creationTimestamp: {
            seconds: 1640995200, // 2022-01-01
          },
        },
        status: {
          state: 'SUCCESS',
        },
      },
    });

    render(
      <EntityDetailRoute phases={testPhases} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/train/runs/run-123' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    // Should render tabs even with unknown types
    expect(screen.getByText('Unknown Type')).toBeInTheDocument();
    expect(screen.getByText('Execution')).toBeInTheDocument();

    expect(screen.getByText("Page type 'some-unknown-type' not yet supported")).toBeInTheDocument();
  });

  test('handles empty pages array', async () => {
    const testPhases = {
      train: buildPhase({
        id: 'train',
        entities: [
          buildEntity({
            views: [
              {
                type: 'detail',
                metadata: [{ id: 'status.state', label: 'State', type: CellType.STATE }],
                pages: [],
              },
            ],
          }),
        ],
      }),
    };

    const mockRequest = vi.fn().mockResolvedValue({
      pipelineRun: {
        metadata: {
          creationTimestamp: {
            seconds: 1640995200, // 2022-01-01
          },
        },
        status: {
          state: 'SUCCESS',
        },
      },
    });

    render(
      <EntityDetailRoute phases={testPhases} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/train/runs/run-123' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    // Should still render header and metadata
    expect(screen.getByText('Pipeline Runs')).toBeInTheDocument();
    await screen.findByText('Success');

    expect(screen.getByText('No tabs available')).toBeInTheDocument();
  });

  test('redirects to first tab if entityTab is invalid', async () => {
    const testPhases = {
      train: buildPhase({
        id: 'train',
        entities: [
          buildEntity({
            views: [
              {
                type: 'detail',
                metadata: [{ id: 'status.state', label: 'State', type: CellType.STATE }],
                pages: [
                  {
                    id: 'execution',
                    label: 'Execution',
                    ...buildExecutionSchema(),
                  },
                ],
              },
            ],
          }),
        ],
      }),
    };

    const mockRequest = vi.fn().mockResolvedValue({
      pipelineRun: {
        metadata: {
          creationTimestamp: {
            seconds: 1640995200, // 2022-01-01
          },
        },
        status: {
          state: 'SUCCESS',
        },
      },
    });

    render(
      <EntityDetailRoute phases={testPhases} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/train/runs/run-123/invalid-tab' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    await screen.findByText('Execution');
    await screen.findByText('Current pathname: /myproject/train/runs/run-123/execution');
  });

  test('handles error when entity not found', async () => {
    const testPhases = {
      train: buildPhase({
        id: 'train',
        entities: [
          buildEntity({
            views: [
              {
                type: 'detail',
                metadata: [{ id: 'status.state', label: 'State', type: CellType.STATE }],
                pages: [
                  {
                    id: 'execution',
                    label: 'Execution',
                    ...buildExecutionSchema(),
                  },
                ],
              },
            ],
          }),
        ],
      }),
    };

    const mockRequest = vi.fn().mockRejectedValue(new Error('Entity not found'));

    render(
      <EntityDetailRoute phases={testPhases} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({
          location: '/myproject/train/runs/run-123',
        }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    await screen.findByText('Entity not found');
    expect(screen.getByRole('button', { name: /Back to list/i })).toBeInTheDocument();
  });
});
