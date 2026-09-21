import { render, screen } from '@testing-library/react';
import { vi } from 'vitest';

import { TRAIN_PHASE } from '#core/config/phases/train';
import { PhaseListRoute } from '#core/router/phase-list-route';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { getServiceProviderWrapper } from '#core/test/wrappers/get-service-provider-wrapper';

describe('Model list page', () => {
  it('renders the column headers', async () => {
    render(
      <PhaseListRoute phases={{ train: TRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/train/models' }),
        getServiceProviderWrapper({
          request: vi.fn().mockResolvedValue({ modelList: { items: [] } }),
        }),
      ])
    );

    expect(await screen.findByRole('columnheader', { name: 'Model' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Environment' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Model Family' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Type' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Last Updated' })).toBeInTheDocument();
  });

  it('renders resolved environment and type labels for a model row', async () => {
    render(
      <PhaseListRoute phases={{ train: TRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/train/models' }),
        getServiceProviderWrapper({
          request: vi.fn().mockResolvedValue({
            modelList: {
              items: [
                {
                  metadata: {
                    name: 'fraud-classifier',
                    labels: { 'michelangelo/environment': 'production' },
                    creationTimestamp: { seconds: 1700000000 },
                  },
                  spec: {
                    description: 'model workflow=fraud-classifier git=abc123',
                    kind: 'MODEL_KIND_BINARY_CLASSIFICATION',
                    modelFamily: { name: 'fraud-family' },
                  },
                },
                {
                  metadata: {
                    name: 'demand-forecaster',
                    labels: { 'michelangelo/environment': 'development' },
                    creationTimestamp: { seconds: 1700000000 },
                  },
                  spec: {
                    description: 'model workflow=demand-forecaster git=def456',
                    kind: 'MODEL_KIND_REGRESSION',
                    modelFamily: { name: 'demand-family' },
                  },
                },
                {
                  metadata: {
                    name: 'user-segmenter',
                    labels: { 'michelangelo/environment': 'testing' },
                    creationTimestamp: { seconds: 1700000000 },
                  },
                  spec: {
                    description: 'model workflow=user-segmenter git=ghi789',
                    kind: 'MODEL_KIND_CLUSTERING',
                    modelFamily: { name: 'segment-family' },
                  },
                },
              ],
            },
          }),
        }),
      ])
    );

    expect(await screen.findByRole('link', { name: 'fraud-classifier' })).toBeInTheDocument();
    expect(screen.getByText('Production')).toBeInTheDocument();
    expect(screen.getByText('Binary Classification')).toBeInTheDocument();
    expect(screen.getByText('fraud-family')).toBeInTheDocument();
    expect(screen.getByText('model workflow=fraud-classifier git=abc123')).toBeInTheDocument();

    expect(screen.getByRole('link', { name: 'demand-forecaster' })).toBeInTheDocument();
    expect(screen.getByText('Development')).toBeInTheDocument();
    expect(screen.getByText('Regression')).toBeInTheDocument();

    expect(screen.getByRole('link', { name: 'user-segmenter' })).toBeInTheDocument();
    expect(screen.getByText('Clustering')).toBeInTheDocument();
  });
});
