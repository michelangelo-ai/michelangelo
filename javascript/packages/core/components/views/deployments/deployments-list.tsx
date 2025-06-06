import React from 'react';
import { Block } from 'baseui/block';
import { HeadingXLarge, HeadingMedium } from 'baseui/typography';
import { Button } from 'baseui/button';
import { Table } from 'baseui/table-semantic';
import { Tag } from 'baseui/tag';
import { useStudioQuery } from '#core/hooks/use-studio-query';

export function DeploymentsList() {
  const { data, isLoading } = useStudioQuery<{ deployments: any[] }>({
    queryName: 'ListDeployments',
    serviceOptions: { namespace: 'default' },
  });

  // Mock data for demonstration
  const deployments = data?.deployments || [
    {
      id: 'bert-cola-deployment',
      name: 'BERT-CoLA Deployment',
      model: 'bert-cola-13',
      status: 'HEALTHY',
      stage: 'ROLLOUT_COMPLETE',
      endpoint: 'bert-cola-endpoint',
      traffic: '100%',
      created: '2025-01-15T10:30:00Z',
      lastUpdated: '2025-01-15T12:45:00Z'
    },
    {
      id: 'sentiment-analyzer',
      name: 'Sentiment Analyzer',
      model: 'roberta-sentiment-v2',
      status: 'DEPLOYING',
      stage: 'ROLLOUT_IN_PROGRESS',
      endpoint: 'sentiment-api',
      traffic: '50%',
      created: '2025-01-15T11:00:00Z',
      lastUpdated: '2025-01-15T11:30:00Z'
    },
    {
      id: 'image-classifier',
      name: 'Image Classifier',
      model: 'resnet50-v3',
      status: 'UNHEALTHY',
      stage: 'ROLLOUT_FAILED',
      endpoint: 'image-api',
      traffic: '0%',
      created: '2025-01-14T15:20:00Z',
      lastUpdated: '2025-01-14T16:00:00Z'
    }
  ];

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'HEALTHY': return 'positive';
      case 'DEPLOYING': return 'warning';
      case 'UNHEALTHY': return 'negative';
      default: return 'neutral';
    }
  };

  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleString();
  };

  return (
    <Block padding="scale800">
      <Block display="flex" justifyContent="space-between" alignItems="center" marginBottom="scale800">
        <HeadingXLarge>Deployments</HeadingXLarge>
        <Button size="default">
          Deploy New Model
        </Button>
      </Block>

      <Block marginBottom="scale600">
        <HeadingMedium marginBottom="scale400">Active Deployments</HeadingMedium>
        
        <Table
          columns={[
            'Name',
            'Model',
            'Status',
            'Stage', 
            'Endpoint',
            'Traffic',
            'Last Updated',
            'Actions'
          ]}
          data={deployments.map(deployment => [
            deployment.name,
            deployment.model,
            <Tag
              key="status"
              kind={getStatusColor(deployment.status)}
              closeable={false}
            >
              {deployment.status}
            </Tag>,
            deployment.stage.replace(/_/g, ' '),
            deployment.endpoint,
            deployment.traffic,
            formatDate(deployment.lastUpdated),
            <Block key="actions" display="flex" gridGap="scale200">
              <Button size="mini" kind="secondary">
                View
              </Button>
              <Button size="mini" kind="secondary">
                Logs
              </Button>
              <Button size="mini" kind="tertiary">
                Scale
              </Button>
            </Block>
          ])}
          overrides={{
            TableHeadCell: {
              style: {
                backgroundColor: '#F8FAFC',
                fontWeight: 600,
              }
            },
            TableBodyRow: {
              style: {
                ':hover': {
                  backgroundColor: '#F8FAFC',
                }
              }
            }
          }}
        />
      </Block>

      {/* Deployment Metrics Summary */}
      <Block
        padding="scale600"
        backgroundColor="#F8FAFC"
        borderRadius="8px"
        marginTop="scale600"
      >
        <HeadingMedium marginBottom="scale400">Deployment Summary</HeadingMedium>
        <Block display="flex" gridGap="scale800">
          <Block>
            <Block color="contentSecondary">Total Deployments</Block>
            <Block font="font600">{deployments.length}</Block>
          </Block>
          <Block>
            <Block color="contentSecondary">Healthy</Block>
            <Block font="font600" color="positive">
              {deployments.filter(d => d.status === 'HEALTHY').length}
            </Block>
          </Block>
          <Block>
            <Block color="contentSecondary">Deploying</Block>
            <Block font="font600" color="warning">
              {deployments.filter(d => d.status === 'DEPLOYING').length}
            </Block>
          </Block>
          <Block>
            <Block color="contentSecondary">Failed</Block>
            <Block font="font600" color="negative">
              {deployments.filter(d => d.status === 'UNHEALTHY').length}
            </Block>
          </Block>
        </Block>
      </Block>
    </Block>
  );
}