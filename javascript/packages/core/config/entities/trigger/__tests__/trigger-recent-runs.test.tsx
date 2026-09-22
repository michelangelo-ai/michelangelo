import { render, screen, waitFor } from '@testing-library/react';

import { TRIGGERED_BY_LABEL } from '#core/config/entities/run/shared';
import { RETRAIN_PHASE } from '#core/config/phases/retrain';
import { EntityDetailRoute } from '#core/router/entity-detail-route';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import {
  createQueryMockRouter,
  getServiceProviderWrapper,
} from '#core/test/wrappers/get-service-provider-wrapper';

describe('Trigger detail "Recent Runs"', () => {
  const SELECTOR = `${TRIGGERED_BY_LABEL}=nightly-trigger`;

  /**
   * The runs this trigger produced are found only through the label selector. The
   * storage layer drops `listOptions.labelSelector` outright if a caller ever also
   * sets `listOptionsExt.operation` (go/storage/mysql/mysql.go), which would silently
   * turn this tab into a list of every run in the namespace. Pin the exact request.
   */
  it('lists runs filtered by the triggered-by label for this trigger', async () => {
    const request = createQueryMockRouter({
      GetTriggerRun: {
        triggerRun: {
          metadata: { name: 'nightly-trigger', namespace: 'myproject' },
          spec: { pipeline: { name: 'my-pipeline', namespace: 'myproject' } },
          status: { state: 1 },
        },
      },
      [`ListPipelineRun:{"listOptions":{"labelSelector":"${SELECTOR}"},"namespace":"myproject"}`]: {
        pipelineRunList: {
          items: [
            {
              metadata: { name: 'run-1', labels: { [TRIGGERED_BY_LABEL]: 'nightly-trigger' } },
              status: { state: 3 },
            },
          ],
        },
      },
    });

    render(
      <EntityDetailRoute phases={{ retrain: RETRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/retrain/triggers/nightly-trigger' }),
        getServiceProviderWrapper({ request }),
      ])
    );

    await waitFor(() =>
      expect(request).toHaveBeenCalledWith(
        'ListPipelineRun',
        { namespace: 'myproject', listOptions: { labelSelector: SELECTOR } },
        {}
      )
    );

    await screen.findByRole('row', { name: /run-1/ });
  });

  it('omits the redundant "Triggered by" column, since every row shares this trigger', async () => {
    render(
      <EntityDetailRoute phases={{ retrain: RETRAIN_PHASE }} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/myproject/retrain/triggers/nightly-trigger' }),
        getServiceProviderWrapper({
          request: createQueryMockRouter({
            GetTriggerRun: {
              triggerRun: {
                metadata: { name: 'nightly-trigger', namespace: 'myproject' },
                spec: { pipeline: { name: 'my-pipeline', namespace: 'myproject' } },
                status: { state: 1 },
              },
            },
            [`ListPipelineRun:{"listOptions":{"labelSelector":"${SELECTOR}"},"namespace":"myproject"}`]:
              {
                pipelineRunList: {
                  items: [
                    {
                      metadata: {
                        name: 'run-1',
                        labels: { [TRIGGERED_BY_LABEL]: 'nightly-trigger' },
                      },
                      status: { state: 3 },
                    },
                  ],
                },
              },
          }),
        }),
      ])
    );

    await screen.findByRole('row', { name: /run-1/ });
    expect(screen.queryByRole('columnheader', { name: 'Triggered by' })).not.toBeInTheDocument();
  });
});
