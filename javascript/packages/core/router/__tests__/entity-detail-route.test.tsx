import { render, screen } from '@testing-library/react';
import { vi } from 'vitest';

import { CellType } from '#core/components/cell/constants';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { getServiceProviderWrapper } from '#core/test/wrappers/get-service-provider-wrapper';
import {
  buildEntityConfigFactory,
  buildPhaseConfigFactory,
} from '../__fixtures__/phase-config-factory';
import { EntityDetailRoute } from '../entity-detail-route';

import type { PhaseConfig } from '#core/types/common/studio-types';

describe('EntityDetailRoute', () => {
  const buildEntity = buildEntityConfigFactory();
  const buildPhase = buildPhaseConfigFactory();

  const testPhaseEntityConfig: Record<string, PhaseConfig> = {
    train: buildPhase({
      id: 'train',
      entities: [
        buildEntity({
          id: 'runs',
          name: 'Pipeline Runs',
          service: 'pipelineRun',
          views: [
            {
              type: 'detail',
              metadata: [
                { id: 'metadata.creationTimestamp.seconds', label: 'Created', type: CellType.DATE },
                { id: 'status.state', label: 'State', type: CellType.STATE },
              ],
              pages: [{ type: 'execution' }],
            },
          ],
        }),
      ],
    }),
  };

  test('renders complete detail view with header', async () => {
    const mockEntityData = {
      pipelineRun: {
        metadata: {
          creationTimestamp: {
            seconds: 1640995200, // 2022-01-01
          },
        },
        status: {
          state: 'RUNNING',
        },
      },
    };
    const mockRequest = vi.fn().mockResolvedValue(mockEntityData);

    render(
      <EntityDetailRoute phases={testPhaseEntityConfig} />,
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

    expect(screen.getByText('execution will go here...')).toBeInTheDocument();
  });

  test('handles error when entity not found', async () => {
    const mockRequest = vi.fn().mockRejectedValue(new Error('Entity not found'));

    render(
      <EntityDetailRoute phases={testPhaseEntityConfig} />,
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
