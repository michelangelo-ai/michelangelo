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
    it('renders header metadata for the model', async () => {
      render(
        <EntityDetailRoute phases={{ train: TRAIN_PHASE }} />,
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper({
            location: '/myproject/train/models/fraud-classifier/information',
          }),
          getServiceProviderWrapper({
            request: createQueryMockRouter({
              ListDeployment: { deploymentList: { items: [] } },
              GetModel: {
                model: {
                  metadata: {
                    name: 'fraud-classifier',
                    creationTimestamp: { seconds: 1700000000 },
                  },
                  spec: {
                    owner: { name: 'jsmith' },
                    // The generated proto client decodes enum fields to their numeric
                    // discriminant (MODEL_KIND_BINARY_CLASSIFICATION = 3), not the enum's
                    // string name.
                    kind: 3,
                    sourcePipelineRun: { name: 'fraud-classifier-run-1' },
                    description: 'Fraud detection model trained on transaction history.',
                  },
                },
              },
            }),
          }),
        ])
      );

      expect(screen.getByText('fraud-classifier')).toBeInTheDocument();
      expect(await screen.findByText('Source pipeline run')).toBeInTheDocument();
      for (const link of screen.getAllByRole('link', { name: 'fraud-classifier-run-1' })) {
        expect(link).toHaveAttribute('href', '/myproject/train/runs/fraud-classifier-run-1');
      }
      expect(screen.getByText('Trained by')).toBeInTheDocument();
      // 'Creation time' and 'Type' also appear as column headers of the deployments table.
      expect(screen.getAllByText('Creation time').length).toBeGreaterThan(0);
      expect(screen.getByText('Last updated')).toBeInTheDocument();
      expect(screen.getAllByText('Type').length).toBeGreaterThan(0);
      expect(screen.getByText('Binary Classification')).toBeInTheDocument();
    });
  });

  describe('information tab', () => {
    const DEPLOYED_TO_ONLINE = {
      metadata: { name: 'fraud-classifier-prod', creationTimestamp: { seconds: 1700000000 } },
      spec: {
        definition: { type: 'TARGET_TYPE_INFERENCE_SERVER' },
        target: { case: 'inferenceServer', value: { name: 'ma-endpoint-fraud' } },
        owner: { name: 'adoe' },
      },
      status: {
        // DEPLOYMENT_STAGE_ROLLOUT_COMPLETE = 4, DEPLOYMENT_STATE_HEALTHY = 2
        stage: 4,
        state: 2,
        currentRevision: { name: 'fraud-classifier' },
      },
    };

    function renderInformationTab({ deployments = [] }: { deployments?: object[] } = {}) {
      const request = createQueryMockRouter({
        ListDeployment: { deploymentList: { items: deployments } },
        GetModel: {
          model: {
            metadata: {
              name: 'fraud-classifier',
              creationTimestamp: { seconds: 1700000000 },
            },
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
          },
        },
      });
      render(
        <EntityDetailRoute phases={{ train: TRAIN_PHASE }} />,
        buildWrapper([
          getErrorProviderWrapper(),
          getRouterWrapper({
            location: '/myproject/train/models/fraud-classifier/information',
          }),
          getServiceProviderWrapper({ request }),
        ])
      );
      return { request };
    }

    it('renders the source pipeline run link in Useful links', async () => {
      renderInformationTab();

      for (const link of await screen.findAllByRole('link', { name: 'fraud-classifier-run-1' })) {
        expect(link).toHaveAttribute('href', '/myproject/train/runs/fraud-classifier-run-1');
      }
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

    it('lists deployments whose current revision is this model', async () => {
      const { request } = renderInformationTab({ deployments: [DEPLOYED_TO_ONLINE] });

      expect(await screen.findByText('Key status indicators')).toBeInTheDocument();
      expect(
        screen.getByText('Deployments this model is currently deployed to')
      ).toBeInTheDocument();
      const link = await screen.findByRole('link', { name: 'fraud-classifier-prod' });
      expect(link).toHaveAttribute('href', '/myproject/deploy/deployments/fraud-classifier-prod');
      expect(screen.getByText('Online')).toBeInTheDocument();
      expect(screen.getByText('Rollout complete')).toBeInTheDocument();
      expect(screen.getByText('ma-endpoint-fraud')).toBeInTheDocument();
      expect(screen.getByText('adoe')).toBeInTheDocument();
      expect(screen.getByText('Healthy')).toBeInTheDocument();

      // The apiserver maps the status.current_revision index to the current_revision_name
      // storage column; the field selector must use that key, not the proto path.
      expect(request.getCall('ListDeployment')?.args).toMatchObject({
        namespace: 'myproject',
        listOptions: { fieldSelector: 'current_revision_name=fraud-classifier' },
      });
    });

    it('shows an empty state when the model is not deployed', async () => {
      renderInformationTab();

      expect(await screen.findByText('Model is not currently deployed')).toBeInTheDocument();
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
