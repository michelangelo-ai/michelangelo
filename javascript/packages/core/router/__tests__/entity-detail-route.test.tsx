import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import { ActionHierarchy } from '#core/components/actions/types';
import { CellType } from '#core/components/cell/constants';
import { buildTableConfigFactory } from '#core/components/views/__fixtures__/table-config-factory';
import { buildExecutionSchemaFactory } from '#core/components/views/execution/__fixtures__/execution-schema-factory';
import { interpolate } from '#core/interpolation/interpolate';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import {
  createQueryMockRouter,
  getServiceProviderWrapper,
} from '#core/test/wrappers/get-service-provider-wrapper';
import {
  buildEntityConfigFactory,
  buildPhaseConfigFactory,
} from '../__fixtures__/phase-config-factory';
import { EntityDetailRoute } from '../entity-detail-route';

import type { ActionComponentProps } from '#core/components/actions/types';
import type {
  CustomDetailPageConfig,
  TableDetailPageConfig,
} from '#core/components/views/detail-view/types/detail-view-schema-types';

describe('EntityDetailRoute', () => {
  const buildEntity = buildEntityConfigFactory({
    id: 'runs',
    name: 'Pipeline Runs',
    service: 'pipelineRun',
  });
  const buildExecutionSchema = buildExecutionSchemaFactory();
  const buildTableConfig = buildTableConfigFactory();
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

  test('table does not fetch data when query is disabled', async () => {
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
                    id: 'runs-table',
                    label: 'Related Runs',
                    type: 'table',
                    queryConfig: {
                      service: 'pipelineRun',
                      serviceOptions: {},
                      clientOptions: { enabled: false },
                    },
                    tableConfig: buildTableConfig({
                      columns: [{ id: 'name', label: 'Run Name', type: CellType.TEXT }],
                    }),
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
            seconds: 1640995200,
          },
        },
        status: {
          state: 'SUCCESS',
        },
      },
    };

    const mockRequest = createQueryMockRouter({
      GetPipelineRun: mockEntityData,
      ListPipelineRun: { pipelineRunList: { items: [{ name: 'Should Not Appear' }] } },
    });

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

    // Verify detail view loads
    await screen.findByText('Success');
    expect(screen.getByText('Related Runs')).toBeInTheDocument();

    // Verify table query was never called due to enabled: false
    expect(mockRequest).not.toHaveBeenCalledWith('ListPipelineRun', expect.anything());
    expect(screen.queryByText('Should Not Appear')).not.toBeInTheDocument();
  });

  test('table respects custom service options from config', async () => {
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
                    id: 'filtered-runs',
                    label: 'Filtered Runs',
                    type: 'table',
                    queryConfig: {
                      service: 'pipelineRun',
                      serviceOptions: {
                        filter: 'status=SUCCESS',
                        limit: 10,
                      },
                    },
                    tableConfig: buildTableConfig({
                      columns: [{ id: 'name', label: 'Run Name', type: CellType.TEXT }],
                    }),
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
        metadata: { creationTimestamp: { seconds: 1640995200 } },
        status: { state: 'SUCCESS' },
      },
    };

    const mockRequest = createQueryMockRouter({
      GetPipelineRun: mockEntityData,
      'ListPipelineRun:{"filter":"status=SUCCESS","limit":10,"namespace":"myproject"}': {
        pipelineRunList: { items: [{ name: 'Filtered Success Run' }] },
      },
    });

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

    await screen.findByRole('row', { name: /Filtered Success Run/ });
  });

  test('handles table tab request failure gracefully', async () => {
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
                    id: 'runs-table',
                    label: 'Related Runs',
                    type: 'table',
                    queryConfig: {
                      service: 'pipelineRun',
                      endpoint: 'list',
                      serviceOptions: {},
                    },
                    tableConfig: buildTableConfig({
                      columns: [{ id: 'name', label: 'Run Name', type: CellType.TEXT }],
                    }),
                  } as TableDetailPageConfig,
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
            seconds: 1640995200,
          },
        },
        status: {
          state: 'SUCCESS',
        },
      },
    };

    const mockRequest = createQueryMockRouter({
      GetPipelineRun: mockEntityData,
      ListPipelineRun: new Error('Table API Error'),
    });

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

    await screen.findByText('Unable to fetch data for the table');
  });

  test('resolves interpolation at various configuration levels', async () => {
    const testPhases = {
      train: buildPhase({
        id: 'train',
        entities: [
          buildEntity({
            views: [
              {
                type: 'detail',
                metadata: [
                  { id: 'status.state', label: 'State', type: CellType.STATE },
                  {
                    id: 'metadata.name',
                    label: 'Name: ${page.metadata.name}',
                    type: CellType.TEXT,
                  },
                ],
                pages: [
                  {
                    id: 'interpolated-table',
                    label: 'Related Runs',
                    type: 'table',
                    queryConfig: {
                      endpoint: 'list',
                      service: 'pipelineRun',
                      serviceOptions: {
                        listOptions: {
                          labelSelector:
                            'pipelinerun.michelangelo/source-trigger=${page.metadata.name}',
                        },
                      },
                    },
                    tableConfig: buildTableConfig({
                      columns: [
                        {
                          id: 'metadata.name',
                          label: 'Run Name',
                          type: CellType.TEXT,
                          url: '/${studio.projectId}/${studio.phase}/runs/${row.metadata.name}?page=${page.metadata.name}',
                        },
                      ],
                    }),
                  },
                ],
              },
            ],
          }),
        ],
      }),
    };

    const mockRequest = createQueryMockRouter({
      GetPipelineRun: {
        pipelineRun: {
          metadata: {
            name: 'test-trigger-123',
            namespace: 'myproject',
            creationTimestamp: { seconds: 1640995200 },
          },
          status: { state: 'SUCCESS' },
        },
      },
      'ListPipelineRun:{"listOptions":{"labelSelector":"pipelinerun.michelangelo/source-trigger=test-trigger-123"},"namespace":"myproject"}':
        {
          pipelineRunList: {
            items: [
              {
                metadata: { name: 'run-1' },
                status: { state: 'SUCCESS' },
              },
              {
                metadata: { name: 'run-2' },
                status: { state: 'FAILED' },
              },
            ],
          },
        },
    });

    render(
      <EntityDetailRoute phases={testPhases} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({
          location: '/myproject/train/runs/test-trigger-123',
        }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    // Resolve interpolation in metadata label
    await screen.findByText('Name: test-trigger-123');

    // Verify interpolation in serviceOptions worked by checking the table data loads
    await screen.findByRole('row', { name: /run-1/ });
    await screen.findByRole('row', { name: /run-2/ });

    // Verify interpolation of shared page/row data
    expect(screen.getByRole('link', { name: /run-1/ })).toHaveAttribute(
      'href',
      '/myproject/train/runs/run-1?page=test-trigger-123'
    );
    expect(screen.getByRole('link', { name: /run-2/ })).toHaveAttribute(
      'href',
      '/myproject/train/runs/run-2?page=test-trigger-123'
    );
  });

  test('renders entity-level actions in the detail page header', async () => {
    const user = userEvent.setup();
    const RunDialog = ({ isOpen, onClose }: ActionComponentProps) =>
      isOpen ? (
        <div role="dialog">
          Run form <button onClick={onClose}>Close</button>
        </div>
      ) : null;

    const testPhases = {
      train: buildPhase({
        id: 'train',
        entities: [
          buildEntity({
            actions: [
              {
                display: { label: 'Run', icon: 'playerPlay' },
                component: RunDialog,
                hierarchy: ActionHierarchy.PRIMARY,
              },
            ],
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
        metadata: { creationTimestamp: { seconds: 1640995200 } },
        status: { state: 'SUCCESS', steps: [] },
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

    const runButton = await screen.findByRole('button', { name: 'Run' });
    expect(runButton).toBeInTheDocument();

    await user.click(runButton);
    expect(await screen.findByRole('dialog')).toHaveTextContent('Run form');

    await user.click(screen.getByRole('button', { name: 'Close' }));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  test('resolves interpolated action hierarchy in the detail page header', async () => {
    const StubDialog = ({ isOpen }: ActionComponentProps) =>
      isOpen ? <div role="dialog">Stub</div> : null;

    const testPhases = {
      train: buildPhase({
        id: 'train',
        entities: [
          buildEntity({
            actions: [
              {
                display: { label: 'Resume' },
                component: StubDialog,
                hierarchy: interpolate(({ data }) => {
                  const record = data as { status?: { state?: string } } | undefined;
                  return record?.status?.state === 'PAUSED'
                    ? ActionHierarchy.PRIMARY
                    : ActionHierarchy.TERTIARY;
                }),
              },
            ],
            views: [
              {
                type: 'detail',
                metadata: [{ id: 'status.state', label: 'State', type: CellType.STATE }],
                pages: [{ id: 'execution', label: 'Execution', ...buildExecutionSchema() }],
              },
            ],
          }),
        ],
      }),
    };

    const mockRequest = vi.fn().mockResolvedValue({
      pipelineRun: {
        metadata: { creationTimestamp: { seconds: 1640995200 } },
        status: { state: 'PAUSED', steps: [] },
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

    // Hierarchy resolves to PRIMARY → renders as a direct button, not in overflow menu
    expect(await screen.findByRole('button', { name: 'Resume' })).toBeInTheDocument();
  });
});
