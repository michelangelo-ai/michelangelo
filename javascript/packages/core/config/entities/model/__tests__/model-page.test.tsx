import { render, screen } from '@testing-library/react';
import { vi } from 'vitest';

import { TRAIN_PHASE } from '#core/config/phases/train';
import { EntityDetailRoute } from '#core/router/entity-detail-route';
import { PhaseListRoute } from '#core/router/phase-list-route';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import {
  createQueryMockRouter,
  getServiceProviderWrapper,
} from '#core/test/wrappers/get-service-provider-wrapper';

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
                    // The generated proto client decodes enum fields to their numeric
                    // discriminant (MODEL_KIND_BINARY_CLASSIFICATION = 3), not the enum's
                    // string name — mock the real runtime shape, not the wire JSON shape.
                    kind: 3,
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
                    kind: 2, // MODEL_KIND_REGRESSION
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
                    kind: 5, // MODEL_KIND_CLUSTERING
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

describe('Model detail page', () => {
  describe('header metadata row', () => {
    it('renders source pipeline run, owner, timestamps, and type', async () => {
      render(
        <EntityDetailRoute phases={{ train: TRAIN_PHASE }} />,
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper({ location: '/myproject/train/models/fraud-classifier/information' }),
          getServiceProviderWrapper({
            request: createQueryMockRouter({
              GetModel: {
                model: {
                  metadata: {
                    name: 'fraud-classifier',
                    creationTimestamp: { seconds: 1700000000 },
                    labels: { 'michelangelo/SpecUpdateTimestamp': '1700500000' },
                  },
                  spec: {
                    sourcePipelineRun: { name: 'run-42' },
                    owner: { name: 'model-owner' },
                    kind: 3, // MODEL_KIND_BINARY_CLASSIFICATION
                  },
                },
              },
            }),
          }),
        ])
      );

      const runLinks = await screen.findAllByRole('link', { name: 'run-42' });
      expect(runLinks[0]).toHaveAttribute('href', '/myproject/train/runs/run-42');
      expect(screen.getByText('model-owner')).toBeInTheDocument();
      expect(screen.getByText('Binary Classification')).toBeInTheDocument();
    });
  });

  describe('information tab', () => {
    const buildModel = (overrides = {}) => ({
      metadata: { name: 'fraud-classifier' },
      spec: {
        description: 'model workflow=fraud-classifier git=abc123',
        sourcePipelineRun: { name: 'run-42' },
      },
      ...overrides,
    });

    it('renders the model description', async () => {
      render(
        <EntityDetailRoute phases={{ train: TRAIN_PHASE }} />,
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper({ location: '/myproject/train/models/fraud-classifier/information' }),
          getServiceProviderWrapper({
            request: createQueryMockRouter({ GetModel: { model: buildModel() } }),
          }),
        ])
      );

      expect(
        await screen.findByText('model workflow=fraud-classifier git=abc123')
      ).toBeInTheDocument();
    });

    it('shows a fallback message when no description is set', async () => {
      render(
        <EntityDetailRoute phases={{ train: TRAIN_PHASE }} />,
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper({ location: '/myproject/train/models/fraud-classifier/information' }),
          getServiceProviderWrapper({
            request: createQueryMockRouter({
              GetModel: { model: buildModel({ spec: { sourcePipelineRun: { name: 'run-42' } } }) },
            }),
          }),
        ])
      );

      expect(await screen.findByText('No description provided.')).toBeInTheDocument();
    });

    it('renders a link to the source pipeline run', async () => {
      render(
        <EntityDetailRoute phases={{ train: TRAIN_PHASE }} />,
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper({ location: '/myproject/train/models/fraud-classifier/information' }),
          getServiceProviderWrapper({
            request: createQueryMockRouter({ GetModel: { model: buildModel() } }),
          }),
        ])
      );

      // The source pipeline run link appears both in the page header metadata row and in
      // this tab's "Useful links" section, matching internal's behavior of surfacing it in
      // both places.
      const runLinks = await screen.findAllByRole('link', { name: 'run-42' });
      expect(runLinks.length).toBeGreaterThanOrEqual(1);
      expect(runLinks[0]).toHaveAttribute('href', '/myproject/train/runs/run-42');
    });
  });

  describe('performance tab', () => {
    it('shows a message when the model has no performance report configured', async () => {
      render(
        <EntityDetailRoute phases={{ train: TRAIN_PHASE }} />,
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper({ location: '/myproject/train/models/fraud-classifier/performance' }),
          getServiceProviderWrapper({
            request: createQueryMockRouter({
              GetModel: { model: { metadata: { name: 'fraud-classifier' }, spec: {} } },
            }),
          }),
        ])
      );

      expect(
        await screen.findByText('No performance report is available for this model.')
      ).toBeInTheDocument();
    });

    it('lists chart titles from the fetched evaluation report', async () => {
      render(
        <EntityDetailRoute phases={{ train: TRAIN_PHASE }} />,
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper({ location: '/myproject/train/models/fraud-classifier/performance' }),
          getServiceProviderWrapper({
            request: createQueryMockRouter({
              GetModel: {
                model: {
                  metadata: { name: 'fraud-classifier' },
                  spec: { performanceEvaluationReport: { name: 'report-a' } },
                },
              },
              GetEvaluationReport: {
                evaluationReport: {
                  spec: {
                    title: 'Model Performance',
                    charts: [{ title: 'Accuracy by class' }, { title: 'ROC Curve' }],
                  },
                },
              },
            }),
          }),
        ])
      );

      expect(await screen.findByText('Model Performance')).toBeInTheDocument();
      expect(screen.getByText('Accuracy by class')).toBeInTheDocument();
      expect(screen.getByText('ROC Curve')).toBeInTheDocument();
    });

    it('shows a message when the report has no charts', async () => {
      render(
        <EntityDetailRoute phases={{ train: TRAIN_PHASE }} />,
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper({ location: '/myproject/train/models/fraud-classifier/performance' }),
          getServiceProviderWrapper({
            request: createQueryMockRouter({
              GetModel: {
                model: {
                  metadata: { name: 'fraud-classifier' },
                  spec: { performanceEvaluationReport: { name: 'report-a' } },
                },
              },
              GetEvaluationReport: {
                evaluationReport: { spec: { title: 'Empty Report', charts: [] } },
              },
            }),
          }),
        ])
      );

      expect(await screen.findByText('This report has no charts.')).toBeInTheDocument();
    });
  });
});
