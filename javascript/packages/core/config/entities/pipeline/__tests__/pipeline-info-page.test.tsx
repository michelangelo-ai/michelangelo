import { render, screen } from '@testing-library/react';

import { PIPELINE_ENTITY_CONFIG } from '#core/config/entities/pipeline/pipeline';
import { EntityDetailRoute } from '#core/router/entity-detail-route';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { getInterpolationProviderWrapper } from '#core/test/wrappers/get-interpolation-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import {
  createQueryMockRouter,
  getServiceProviderWrapper,
} from '#core/test/wrappers/get-service-provider-wrapper';
import { getSnackbarProviderWrapper } from '#core/test/wrappers/get-snackbar-provider-wrapper';

describe('Pipeline detail page', () => {
  describe('Information tab', () => {
    function renderInfoTab(pipeline: object) {
      render(
        <EntityDetailRoute
          phases={{
            train: {
              id: 'train',
              icon: 'train',
              name: 'Train',
              state: 'active',
              entities: [PIPELINE_ENTITY_CONFIG],
            },
          }}
        />,
        buildWrapper([
          getBaseProviderWrapper(),
          getErrorProviderWrapper(),
          getIconProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper({ location: '/ma-dev-test/train/pipelines/training-pipeline/info' }),
          getServiceProviderWrapper({
            request: createQueryMockRouter({ GetPipeline: { pipeline } }),
          }),
          getSnackbarProviderWrapper(),
        ])
      );
    }

    it('is the first tab and is selected when routed to', async () => {
      renderInfoTab({
        metadata: { name: 'training-pipeline', namespace: 'ma-dev-test' },
        spec: { owner: { name: 'me' }, manifest: { type: 3 } },
      });

      const tabs = await screen.findAllByRole('tab');
      expect(tabs[0]).toHaveAccessibleName('Information');
      expect(screen.getByRole('tab', { name: 'Information', selected: true })).toBeInTheDocument();
    });

    it('renders the whole manifest as JSON, including triggers and unpacked content', async () => {
      renderInfoTab({
        metadata: { name: 'training-pipeline', namespace: 'ma-dev-test' },
        spec: {
          owner: { name: 'me' },
          manifest: {
            // The generated proto client decodes enum fields to their numeric
            // discriminant (PIPELINE_MANIFEST_TYPE_UNIFLOW = 3), not the enum's string name.
            type: 3,
            filePath: 'examples.bert_cola.bert_cola',
            uniflowTar: 's3://default/bert_local.tar',
            triggerMap: {
              'daily-at-8am': {
                triggerType: { case: 'cronSchedule', value: { cron: '0 8 * * *' } },
              },
              'every-minute': {
                triggerType: { case: 'cronSchedule', value: { cron: '* * * * *' } },
                // Durations decode to protobuf-es Duration messages, whose `seconds` is a bigint;
                // JSON.stringify throws on bigint unless it is converted first.
                batchPolicy: { batchSize: 1, wait: { seconds: BigInt(60), nanos: 0 } },
              },
            },
            content: {
              typeUrl: 'type.googleapis.com/michelangelo.UniFlowConf',
              value: { kwargs: [['name', 'cola']] },
            },
          },
        },
      });

      expect(await screen.findByText('Configuration')).toBeInTheDocument();
      expect(screen.getByText('Manifest')).toBeInTheDocument();
      // The CodeMirror editor exposes its content as a readonly textbox.
      const editor = await screen.findByRole('textbox');
      expect(editor).toHaveTextContent('"filePath": "examples.bert_cola.bert_cola"');
      expect(editor).toHaveTextContent('"cron": "0 8 * * *"');
      expect(editor).toHaveTextContent('"seconds": "60"');
      expect(editor).toHaveTextContent('"typeUrl": "type.googleapis.com/michelangelo.UniFlowConf"');
    });

    it('shows an empty state when the pipeline has no manifest', async () => {
      renderInfoTab({
        metadata: { name: 'training-pipeline', namespace: 'ma-dev-test' },
        spec: { owner: { name: 'me' } },
      });

      expect(await screen.findByText('No manifest available')).toBeInTheDocument();
      expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
    });
  });
});
