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

    it('renders header metadata for the model', async () => {
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

      expect(screen.getByText('fraud-classifier')).toBeInTheDocument();
      expect(await screen.findByText('Source pipeline run')).toBeInTheDocument();
      for (const link of screen.getAllByRole('link', { name: 'fraud-classifier-run-1' })) {
        expect(link).toHaveAttribute('href', '/myproject/train/runs/fraud-classifier-run-1');
      }
      expect(screen.getByText('Trained by')).toBeInTheDocument();
      expect(screen.getByText('Creation time')).toBeInTheDocument();
      expect(screen.getByText('Last updated')).toBeInTheDocument();
      expect(screen.getByText('Type')).toBeInTheDocument();
      expect(screen.getByText('Binary Classification')).toBeInTheDocument();
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

    beforeEach(() => {
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
    });

    it('renders the source pipeline run link in Useful links', async () => {
      for (const link of await screen.findAllByRole('link', { name: 'fraud-classifier-run-1' })) {
        expect(link).toHaveAttribute('href', '/myproject/train/runs/fraud-classifier-run-1');
      }
    });

    it('renders the description', async () => {
      expect(
        await screen.findByDisplayValue('Fraud detection model trained on transaction history.')
      ).toBeInTheDocument();
    });

    it('renders the model context configuration fields', async () => {
      expect(await screen.findByText('Model context')).toBeInTheDocument();
      expect(await screen.findByRole('textbox', { name: 'Model family' })).toHaveValue(
        'fraud-classifier-family'
      );
      expect(screen.getByRole('textbox', { name: 'Training framework' })).toHaveValue('TensorFlow');
      expect(screen.getByRole('textbox', { name: 'Source platform' })).toHaveValue('canvas');
    });

    it('renders the training setup configuration fields', async () => {
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
