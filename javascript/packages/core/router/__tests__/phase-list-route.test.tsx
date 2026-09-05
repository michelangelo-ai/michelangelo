import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Alert } from 'baseui/icon';
import { vi } from 'vitest';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { getServiceProviderWrapper } from '#core/test/wrappers/get-service-provider-wrapper';
import {
  buildEntityConfigFactory,
  buildPhaseConfigFactory,
} from '../__fixtures__/phase-config-factory';
import { PhaseListRoute } from '../phase-list-route';

describe('PhaseListRoute', () => {
  test('renders tabs for active entities only, filtering out disabled ones', () => {
    const buildEntity = buildEntityConfigFactory();
    const buildPhase = buildPhaseConfigFactory();

    const mockRequest = vi.fn().mockResolvedValue({
      pipelineList: { items: [] },
    });

    render(
      <PhaseListRoute
        phases={{
          train: buildPhase({
            id: 'train',
            entities: [
              buildEntity({ id: 'pipelines', name: 'Pipelines', service: 'pipeline' }),
              buildEntity({ id: 'runs', name: 'Pipeline Runs', service: 'pipelineRun' }),
              buildEntity({
                id: 'disabled-entity',
                name: 'Disabled Entity',
                service: 'disabled',
                state: 'disabled',
              }),
            ],
          }),
        }}
      />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/train/pipelines' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    expect(screen.getByRole('tab', { name: 'Pipelines' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'Pipeline Runs' })).toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: 'Disabled Entity' })).not.toBeInTheDocument();
  });

  test('shows error message for unknown phase', () => {
    const buildPhase = buildPhaseConfigFactory();

    render(
      <PhaseListRoute
        phases={{
          train: buildPhase({ id: 'train' }),
          evaluate: buildPhase({ id: 'evaluate' }),
        }}
      />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/unknown-phase/entity' }),
        getServiceProviderWrapper({ request: vi.fn() }),
      ])
    );

    expect(screen.getByText(/Phase "unknown-phase" configuration not found/)).toBeInTheDocument();
    expect(screen.getByText(/Available phases: train, evaluate/)).toBeInTheDocument();
  });

  test('shows message when phase has no listable entities', () => {
    const buildEntity = buildEntityConfigFactory();
    const buildPhase = buildPhaseConfigFactory();

    const configWithNoListableEntities = {
      'no-listable': buildPhase({
        id: 'no-listable',
        entities: [
          buildEntity({
            id: 'disabled-only',
            name: 'Disabled Only',
            service: 'disabled',
            state: 'disabled',
          }),
        ],
      }),
    };

    render(
      <PhaseListRoute phases={configWithNoListableEntities} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/no-listable/entity' }),
        getServiceProviderWrapper({ request: vi.fn() }),
      ])
    );

    expect(
      screen.getByText(/Phase "no-listable" has no active entities with list views configured/)
    ).toBeInTheDocument();
  });

  test('redirects to first entity when no entity in URL', async () => {
    const buildEntity = buildEntityConfigFactory();
    const buildPhase = buildPhaseConfigFactory();

    const mockRequest = vi.fn().mockResolvedValue({
      pipelineList: { items: [] },
    });

    render(
      <PhaseListRoute
        phases={{
          train: buildPhase({
            id: 'train',
            entities: [buildEntity({ id: 'pipelines', name: 'Pipelines', service: 'pipeline' })],
          }),
        }}
      />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/train' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    await screen.findByRole('tab', { name: 'Pipelines' });
  });

  test('shows correct tab as active when entity in URL', () => {
    const buildEntity = buildEntityConfigFactory();
    const buildPhase = buildPhaseConfigFactory();

    const mockRequest = vi.fn().mockResolvedValue({
      pipelineRunList: { items: [] },
    });

    render(
      <PhaseListRoute
        phases={{
          train: buildPhase({
            id: 'train',
            entities: [
              buildEntity({ id: 'pipelines', name: 'Pipelines', service: 'pipeline' }),
              buildEntity({ id: 'runs', name: 'Pipeline Runs', service: 'pipelineRun' }),
            ],
          }),
        }}
      />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/train/runs' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    expect(screen.getByRole('tab', { name: 'Pipelines' })).toHaveAttribute(
      'aria-selected',
      'false'
    );
    expect(screen.getByRole('tab', { name: 'Pipeline Runs' })).toHaveAttribute(
      'aria-selected',
      'true'
    );
  });

  test('shows error message for invalid entity in URL', () => {
    const buildEntity = buildEntityConfigFactory();
    const buildPhase = buildPhaseConfigFactory();

    const mockRequest = vi.fn().mockResolvedValue({
      pipelineList: { items: [] },
    });

    render(
      <PhaseListRoute
        phases={{
          train: buildPhase({
            id: 'train',
            entities: [buildEntity({ id: 'pipelines', name: 'Pipelines', service: 'pipeline' })],
          }),
        }}
      />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/train/invalid-entity' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    expect(screen.getByText(/Entity ".*" not found/)).toBeInTheDocument();
  });

  test('handles data fetching failures gracefully', async () => {
    const buildEntity = buildEntityConfigFactory();
    const buildPhase = buildPhaseConfigFactory();

    const mockRequest = vi.fn().mockRejectedValue(new Error('Network error'));

    render(
      <PhaseListRoute
        phases={{
          train: buildPhase({
            id: 'train',
            entities: [buildEntity({ id: 'pipelines', name: 'Pipelines', service: 'pipeline' })],
          }),
        }}
      />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/train/pipelines' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    expect(screen.getByRole('tab', { name: 'Pipelines' })).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByText('Unable to fetch data for the table')).toBeInTheDocument();
    });
  });

  test('switches tabs and updates data when clicking different tabs', async () => {
    const buildEntity = buildEntityConfigFactory();
    const buildPhase = buildPhaseConfigFactory();
    const user = userEvent.setup();

    const mockRequest = vi
      .fn()
      .mockResolvedValueOnce({
        pipelineList: {
          items: [{ metadata: { name: 'pipeline-1' }, status: 'running' }],
        },
      })
      .mockResolvedValueOnce({
        pipelineRunList: {
          items: [{ metadata: { name: 'run-1' }, status: 'completed' }],
        },
      });

    render(
      <PhaseListRoute
        phases={{
          train: buildPhase({
            id: 'train',
            entities: [
              buildEntity({ id: 'pipelines', name: 'Pipelines', service: 'pipeline' }),
              buildEntity({ id: 'runs', name: 'Pipeline Runs', service: 'pipelineRun' }),
            ],
          }),
        }}
      />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/train/pipelines' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    await waitFor(() => {
      expect(screen.getByText('pipeline-1')).toBeInTheDocument();
    });

    await user.click(screen.getByRole('tab', { name: 'Pipeline Runs' }));

    await waitFor(() => {
      expect(screen.getByText('run-1')).toBeInTheDocument();
      expect(screen.queryByText('pipeline-1')).not.toBeInTheDocument();
    });
  });

  test('handles empty configuration object', () => {
    render(
      <PhaseListRoute phases={{}} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/train/pipelines' }),
        getServiceProviderWrapper({ request: vi.fn() }),
      ])
    );

    expect(screen.getByText(/Phase "train" configuration not found/)).toBeInTheDocument();
    expect(screen.getByText(/Available phases:$/)).toBeInTheDocument();
  });

  test('handles configuration with empty entity arrays', () => {
    const buildPhase = buildPhaseConfigFactory();

    const emptyConfig = {
      train: buildPhase({ id: 'train', entities: [] }),
      evaluate: buildPhase({ id: 'evaluate', entities: [] }),
    };

    render(
      <PhaseListRoute phases={emptyConfig} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/train/pipelines' }),
        getServiceProviderWrapper({ request: vi.fn() }),
      ])
    );

    expect(
      screen.getByText(/Phase "train" has no active entities with list views configured/)
    ).toBeInTheDocument();
  });

  test('filters out entities without list views', () => {
    const buildEntity = buildEntityConfigFactory();
    const buildPhase = buildPhaseConfigFactory();

    const configWithNonListViews = {
      train: buildPhase({
        id: 'train',
        entities: [
          buildEntity({
            id: 'detail-only',
            name: 'Detail Only',
            service: 'detail',
            views: [],
          }),
          buildEntity({
            id: 'pipelines',
            name: 'Pipelines',
            service: 'pipeline',
          }),
        ],
      }),
    };

    const mockRequest = vi.fn().mockResolvedValue({
      pipelineList: { items: [] },
    });

    render(
      <PhaseListRoute phases={configWithNonListViews} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/train/pipelines' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    expect(screen.getByRole('tab', { name: 'Pipelines' })).toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: 'Detail Only' })).not.toBeInTheDocument();
  });

  test('handles empty entities array gracefully', () => {
    const buildPhase = buildPhaseConfigFactory();

    render(
      <PhaseListRoute phases={{ train: buildPhase({ id: 'train', entities: [] }) }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/train' }),
        getServiceProviderWrapper({ request: vi.fn() }),
      ])
    );

    expect(screen.queryByRole('tab')).not.toBeInTheDocument();
  });

  test('handles single entity', () => {
    const buildEntity = buildEntityConfigFactory();
    const buildPhase = buildPhaseConfigFactory();

    const singleEntityConfig = {
      train: buildPhase({
        id: 'train',
        entities: [
          buildEntity({
            id: 'pipelines',
            name: 'Pipelines',
            service: 'pipeline',
          }),
        ],
      }),
    };

    const mockRequest = vi.fn().mockResolvedValue({
      pipelineList: { items: [] },
    });

    render(
      <PhaseListRoute phases={singleEntityConfig} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/train/pipelines' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    expect(screen.getByRole('tab', { name: 'Pipelines' })).toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: 'Pipeline Runs' })).not.toBeInTheDocument();
  });

  test('renders phase header with config metadata', () => {
    const buildEntity = buildEntityConfigFactory();
    const buildPhase = buildPhaseConfigFactory();

    const configWithDescription = {
      train: buildPhase({
        id: 'train',
        icon: 'train',
        name: 'Train & Evaluate',
        description: 'Train machine learning models and evaluate their performance',
        docUrl: 'https://docs.example.com',
        entities: [
          buildEntity({
            id: 'pipelines',
            name: 'Pipelines',
            service: 'pipeline',
          }),
        ],
      }),
    };

    const mockRequest = vi.fn().mockResolvedValue({
      pipelineList: { items: [] },
    });

    render(
      <PhaseListRoute phases={configWithDescription} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/train/pipelines' }),
        getServiceProviderWrapper({ request: mockRequest }),
        getIconProviderWrapper({ icons: { train: Alert } }),
      ])
    );

    expect(screen.getByRole('img', { name: 'Alert' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Train & Evaluate' })).toBeInTheDocument();
    expect(
      screen.getByText('Train machine learning models and evaluate their performance')
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Learn more' })).toBeInTheDocument();
  });
});
