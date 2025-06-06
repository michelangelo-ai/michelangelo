import React from 'react';
import { Block } from 'baseui/block';
import { HeadingXLarge, HeadingMedium, ParagraphSmall } from 'baseui/typography';
import { Button } from 'baseui/button';
import { Card } from 'baseui/card';
import { Grid, Cell } from 'baseui/layout-grid';
import { Tag } from 'baseui/tag';
import { Avatar } from 'baseui/avatar';
import { useStudioQuery } from '#core/hooks/use-studio-query';

export function ModelsList() {
  const { data, isLoading } = useStudioQuery<{ models: any[] }>({
    queryName: 'ListModels',
    serviceOptions: { namespace: 'default' },
  });

  // Mock data for demonstration
  const models = data?.models || [
    {
      id: 'bert-cola-13',
      name: 'BERT-CoLA v13',
      description: 'BERT model fine-tuned for CoLA (Corpus of Linguistic Acceptability) task',
      framework: 'PyTorch',
      version: '13',
      algorithm: 'transformer',
      accuracy: 0.847,
      status: 'DEPLOYED',
      deployments: ['bert-cola-deployment'],
      created: '2025-01-15T10:00:00Z',
      size: '438 MB',
      owner: 'badcount'
    },
    {
      id: 'roberta-sentiment-v2',
      name: 'RoBERTa Sentiment v2',
      description: 'RoBERTa model for sentiment analysis with improved accuracy',
      framework: 'PyTorch',
      version: '2',
      algorithm: 'transformer',
      accuracy: 0.923,
      status: 'READY',
      deployments: ['sentiment-analyzer'],
      created: '2025-01-14T16:30:00Z',
      size: '552 MB',
      owner: 'ml-team'
    },
    {
      id: 'resnet50-v3',
      name: 'ResNet-50 v3',
      description: 'Image classification model based on ResNet-50 architecture',
      framework: 'TensorFlow',
      version: '3',
      algorithm: 'cnn',
      accuracy: 0.765,
      status: 'TRAINING',
      deployments: [],
      created: '2025-01-14T12:00:00Z',
      size: '102 MB',
      owner: 'vision-team'
    },
    {
      id: 'xgboost-classifier',
      name: 'XGBoost Classifier',
      description: 'Gradient boosting classifier for tabular data',
      framework: 'XGBoost',
      version: '1',
      algorithm: 'gradient_boosting',
      accuracy: 0.891,
      status: 'READY',
      deployments: [],
      created: '2025-01-13T09:15:00Z',
      size: '15 MB',
      owner: 'data-science'
    }
  ];

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'DEPLOYED': return 'positive';
      case 'READY': return 'primary';
      case 'TRAINING': return 'warning';
      case 'FAILED': return 'negative';
      default: return 'neutral';
    }
  };

  const getFrameworkColor = (framework: string) => {
    switch (framework) {
      case 'PyTorch': return '#EE4C2C';
      case 'TensorFlow': return '#FF6F00';
      case 'XGBoost': return '#337AB7';
      default: return '#6B7280';
    }
  };

  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleDateString();
  };

  return (
    <Block padding="scale800">
      <Block display="flex" justifyContent="space-between" alignItems="center" marginBottom="scale800">
        <HeadingXLarge>Model Registry</HeadingXLarge>
        <Button size="default">
          Register New Model
        </Button>
      </Block>

      <Grid gridColumns={[1, 2, 3]} gridGaps={[16, 16, 24]}>
        {models.map((model) => (
          <Cell key={model.id}>
            <Card
              overrides={{
                Root: {
                  style: {
                    border: '1px solid #E2E8F0',
                    ':hover': {
                      boxShadow: '0 4px 16px rgba(0, 0, 0, 0.12)',
                    }
                  }
                }
              }}
            >
              <Block marginBottom="scale400">
                <Block display="flex" justifyContent="space-between" alignItems="flex-start" marginBottom="scale300">
                  <Block>
                    <HeadingMedium margin="0" marginBottom="scale200">
                      {model.name}
                    </HeadingMedium>
                    <Block display="flex" alignItems="center" gridGap="scale200" marginBottom="scale300">
                      <Avatar
                        name={model.framework.charAt(0)}
                        size="scale600"
                        overrides={{
                          Avatar: {
                            style: {
                              backgroundColor: getFrameworkColor(model.framework),
                              fontSize: '12px',
                            }
                          }
                        }}
                      />
                      <ParagraphSmall margin="0" color="contentSecondary">
                        {model.framework}
                      </ParagraphSmall>
                    </Block>
                  </Block>
                  <Tag
                    kind={getStatusColor(model.status)}
                    closeable={false}
                  >
                    {model.status}
                  </Tag>
                </Block>

                <ParagraphSmall margin="0" marginBottom="scale400" color="contentSecondary">
                  {model.description}
                </ParagraphSmall>

                <Block marginBottom="scale400">
                  <Block display="flex" justifyContent="space-between" marginBottom="scale200">
                    <ParagraphSmall margin="0" color="contentSecondary">Accuracy</ParagraphSmall>
                    <ParagraphSmall margin="0" font="font600">{(model.accuracy * 100).toFixed(1)}%</ParagraphSmall>
                  </Block>
                  <Block display="flex" justifyContent="space-between" marginBottom="scale200">
                    <ParagraphSmall margin="0" color="contentSecondary">Size</ParagraphSmall>
                    <ParagraphSmall margin="0" font="font600">{model.size}</ParagraphSmall>
                  </Block>
                  <Block display="flex" justifyContent="space-between" marginBottom="scale200">
                    <ParagraphSmall margin="0" color="contentSecondary">Version</ParagraphSmall>
                    <ParagraphSmall margin="0" font="font600">v{model.version}</ParagraphSmall>
                  </Block>
                  <Block display="flex" justifyContent="space-between" marginBottom="scale200">
                    <ParagraphSmall margin="0" color="contentSecondary">Owner</ParagraphSmall>
                    <ParagraphSmall margin="0" font="font600">{model.owner}</ParagraphSmall>
                  </Block>
                  <Block display="flex" justifyContent="space-between">
                    <ParagraphSmall margin="0" color="contentSecondary">Created</ParagraphSmall>
                    <ParagraphSmall margin="0" font="font600">{formatDate(model.created)}</ParagraphSmall>
                  </Block>
                </Block>

                {model.deployments.length > 0 && (
                  <Block marginBottom="scale400">
                    <ParagraphSmall margin="0" marginBottom="scale200" color="contentSecondary">
                      Active Deployments
                    </ParagraphSmall>
                    {model.deployments.map((deployment) => (
                      <Tag key={deployment} kind="neutral" closeable={false} size="small">
                        {deployment}
                      </Tag>
                    ))}
                  </Block>
                )}

                <Block display="flex" gridGap="scale200">
                  <Button size="compact" kind="secondary">
                    View Details
                  </Button>
                  {model.status === 'READY' && (
                    <Button size="compact" kind="primary">
                      Deploy
                    </Button>
                  )}
                  <Button size="compact" kind="tertiary">
                    Download
                  </Button>
                </Block>
              </Block>
            </Card>
          </Cell>
        ))}
      </Grid>

      {/* Model Registry Summary */}
      <Block
        padding="scale600"
        backgroundColor="#F8FAFC"
        borderRadius="8px"
        marginTop="scale800"
      >
        <HeadingMedium marginBottom="scale400">Registry Summary</HeadingMedium>
        <Block display="flex" gridGap="scale800">
          <Block>
            <Block color="contentSecondary">Total Models</Block>
            <Block font="font600">{models.length}</Block>
          </Block>
          <Block>
            <Block color="contentSecondary">Deployed</Block>
            <Block font="font600" color="positive">
              {models.filter(m => m.status === 'DEPLOYED').length}
            </Block>
          </Block>
          <Block>
            <Block color="contentSecondary">Ready</Block>
            <Block font="font600" color="primary">
              {models.filter(m => m.status === 'READY').length}
            </Block>
          </Block>
          <Block>
            <Block color="contentSecondary">Training</Block>
            <Block font="font600" color="warning">
              {models.filter(m => m.status === 'TRAINING').length}
            </Block>
          </Block>
        </Block>
      </Block>
    </Block>
  );
}