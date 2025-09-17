import { render, screen } from '@testing-library/react';

import { CellType } from '#core/components/cell/constants';
import { buildTableConfigFactory } from '#core/components/views/__fixtures__/table-config-factory';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import {
  createQueryMockRouter,
  getServiceProviderWrapper,
} from '#core/test/wrappers/get-service-provider-wrapper';
import { DetailViewTablePage } from '../detail-view-table-page';

describe('DetailViewTablePage', () => {
  const buildTableConfig = buildTableConfigFactory();

  const defaultProps = {
    queryConfig: {
      endpoint: 'list',
      service: 'pipelineRun',
      serviceOptions: {},
    },
    tableConfig: buildTableConfig({
      columns: [{ id: 'name', label: 'Run Name', type: CellType.TEXT }],
    }),
    pageId: 'test-table',
  };

  test('fetches and displays table data when not loading', async () => {
    const mockRequest = createQueryMockRouter({
      ListPipelineRun: {
        pipelineRunList: { items: [{ name: 'Test Run 1' }, { name: 'Test Run 2' }] },
      },
    });

    render(
      <DetailViewTablePage {...defaultProps} isDetailViewLoading={false} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/project/train/runs' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    await screen.findByRole('row', { name: /Test Run 1/ });
    expect(screen.getByRole('row', { name: /Test Run 2/ })).toBeInTheDocument();
  });

  test('does not fetch table data and shows loading when detail view is loading', () => {
    const mockRequest = createQueryMockRouter({
      ListPipelineRun: {
        pipelineRunList: { items: [{ name: 'Should Not Appear' }] },
      },
    });

    render(
      <DetailViewTablePage {...defaultProps} isDetailViewLoading={true} />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/project/train/runs' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    // Verify table query was never called due to detail view loading
    expect(mockRequest).not.toHaveBeenCalledWith('ListPipelineRun', expect.anything());

    expect(screen.queryByText('Should Not Appear')).not.toBeInTheDocument();
    expect(screen.getByTestId('table-loading-state')).toBeInTheDocument();
  });

  test('respects query clientOptions.enabled when detail view is not loading', () => {
    const mockRequest = createQueryMockRouter({
      ListPipelineRun: {
        pipelineRunList: { items: [{ name: 'Should Not Appear' }] },
      },
    });

    render(
      <DetailViewTablePage
        {...defaultProps}
        isDetailViewLoading={false}
        queryConfig={{
          endpoint: 'list',
          service: 'pipelineRun',
          serviceOptions: {},
          clientOptions: { enabled: false },
        }}
      />,
      buildWrapper([
        getErrorProviderWrapper(),
        getRouterWrapper({ location: '/project/train/runs' }),
        getServiceProviderWrapper({ request: mockRequest }),
      ])
    );

    expect(mockRequest).not.toHaveBeenCalledWith('ListPipelineRun', expect.anything());
    expect(screen.queryByText('Should Not Appear')).not.toBeInTheDocument();
  });
});
