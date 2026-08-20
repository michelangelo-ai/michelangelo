import { render, screen } from '@testing-library/react';

import { TRAIN_PHASE } from '#core/config/phases/train';
import { EntityDetailRoute } from '#core/router/entity-detail-route';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import {
  createQueryMockRouter,
  getServiceProviderWrapper,
} from '#core/test/wrappers/get-service-provider-wrapper';

describe('Model detail page', () => {
  describe('header', () => {
    const buildModel = (overrides: Record<string, unknown> = {}) => ({
      metadata: {
        name: 'fraud-classifier',
        creationTimestamp: { seconds: 1700000000 },
      },
      spec: {
        owner: { name: 'jsmith' },
        // The generated proto client decodes enum fields to their numeric discriminant
        // (MODEL_KIND_BINARY_CLASSIFICATION = 3), not the enum's string name.
        kind: 3,
        sourcePipelineRun: { name: 'fraud-classifier-run-1' },
        description: 'Fraud detection model trained on transaction history.',
      },
      ...overrides,
    });

    function renderDetail(model: object) {
      render(
        <EntityDetailRoute phases={{ train: TRAIN_PHASE }} />,
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper({
            location: '/myproject/train/models/fraud-classifier/information',
          }),
          getServiceProviderWrapper({
            request: createQueryMockRouter({ GetModel: { model } }),
          }),
        ])
      );
    }

    it('renders header metadata for the model', async () => {
      renderDetail(buildModel());

      expect(screen.getByText('fraud-classifier')).toBeInTheDocument();
      expect(await screen.findByText('Source pipeline run')).toBeInTheDocument();
      expect(screen.getByText('Trained by')).toBeInTheDocument();
      expect(screen.getByText('Creation time')).toBeInTheDocument();
      expect(screen.getByText('Last updated')).toBeInTheDocument();
      expect(screen.getByText('Type')).toBeInTheDocument();
      expect(screen.getByText('Binary Classification')).toBeInTheDocument();
      expect(screen.getByText('Data Quality')).toBeInTheDocument();
    });

    it('shows Passed when the overall quality score is truthy', async () => {
      renderDetail(
        buildModel({
          spec: {
            ...buildModel().spec,
            qualityScores: [{ name: 'overall_quality_score', value: 1 }],
          },
        })
      );

      expect(await screen.findByText('Passed')).toBeInTheDocument();
    });

    it('shows Failed when the overall quality score is falsy', async () => {
      renderDetail(
        buildModel({
          spec: {
            ...buildModel().spec,
            qualityScores: [{ name: 'overall_quality_score', value: 0 }],
          },
        })
      );

      expect(await screen.findByText('Failed')).toBeInTheDocument();
    });
  });

  describe('information tab', () => {
    const buildModel = () => ({
      metadata: { name: 'fraud-classifier', creationTimestamp: { seconds: 1700000000 } },
      spec: {
        owner: { name: 'jsmith' },
        sourcePipelineRun: { name: 'fraud-classifier-run-1' },
        description: 'Fraud detection model trained on transaction history.',
        modelFamily: { name: 'fraud-classifier-family' },
        trainingFramework: 'TensorFlow',
        source: 'canvas',
        predictionResult: {
          trainTableName: 'fraud_classifier_train_eval',
          testTableName: 'fraud_classifier_validation_eval',
        },
      },
    });

    function renderInformationTab() {
      render(
        <EntityDetailRoute phases={{ train: TRAIN_PHASE }} />,
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper({
            location: '/myproject/train/models/fraud-classifier/information',
          }),
          getServiceProviderWrapper({
            request: createQueryMockRouter({ GetModel: { model: buildModel() } }),
          }),
        ])
      );
    }

    it('renders the source pipeline run link in Useful links', async () => {
      renderInformationTab();

      expect(await screen.findByRole('link', { name: 'fraud-classifier-run-1' })).toHaveAttribute(
        'href',
        '/myproject/train/runs/fraud-classifier-run-1'
      );
    });

    it('renders the description', async () => {
      renderInformationTab();

      expect(
        await screen.findByDisplayValue('Fraud detection model trained on transaction history.')
      ).toBeInTheDocument();
    });

    it('renders the model context configuration fields', async () => {
      renderInformationTab();

      expect(await screen.findByText('Model context')).toBeInTheDocument();
      expect(await screen.findByRole('textbox', { name: 'Model family' })).toHaveValue(
        'fraud-classifier-family'
      );
      expect(screen.getByRole('textbox', { name: 'Training framework' })).toHaveValue('TensorFlow');
      expect(screen.getByRole('textbox', { name: 'Source platform' })).toHaveValue('canvas');
    });

    it('renders the training setup configuration fields', async () => {
      renderInformationTab();

      expect(await screen.findByText('Training setup')).toBeInTheDocument();
      expect(
        await screen.findByRole('textbox', { name: 'Train evaluation Hive table' })
      ).toHaveValue('fraud_classifier_train_eval');
      expect(screen.getByRole('textbox', { name: 'Validation evaluation Hive table' })).toHaveValue(
        'fraud_classifier_validation_eval'
      );
    });
  });
});
