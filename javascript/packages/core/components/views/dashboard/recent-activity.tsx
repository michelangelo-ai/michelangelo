import React from 'react';
import { Block } from 'baseui/block';
import { LabelSmall, ParagraphSmall } from 'baseui/typography';
import { Avatar } from 'baseui/avatar';

export function RecentActivity() {
  // Mock activity data - replace with actual activity feed
  const activities = [
    {
      id: 1,
      type: 'deployment',
      message: 'BERT-CoLA model deployed successfully',
      user: 'system',
      timestamp: '2 minutes ago',
      status: 'success'
    },
    {
      id: 2,
      type: 'model',
      message: 'bert-cola-13 model registered',
      user: 'badcount',
      timestamp: '15 minutes ago',
      status: 'info'
    },
    {
      id: 3,
      type: 'workflow',
      message: 'Training workflow completed',
      user: 'system', 
      timestamp: '1 hour ago',
      status: 'success'
    },
    {
      id: 4,
      type: 'deployment',
      message: 'Traffic routing updated for bert-cola-endpoint',
      user: 'system',
      timestamp: '2 hours ago',
      status: 'info'
    },
    {
      id: 5,
      type: 'model',
      message: 'Model validation completed',
      user: 'system',
      timestamp: '3 hours ago',
      status: 'success'
    }
  ];

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'success': return '#00B44A';
      case 'warning': return '#F59E0B';
      case 'error': return '#EF4444';
      default: return '#1B7CFC';
    }
  };

  const getTypeIcon = (type: string) => {
    switch (type) {
      case 'deployment': return '🚀';
      case 'model': return '🤖';
      case 'workflow': return '⚙️';
      default: return '📝';
    }
  };

  return (
    <Block>
      {activities.map((activity) => (
        <Block
          key={activity.id}
          display="flex"
          alignItems="flex-start"
          marginBottom="scale500"
          paddingBottom="scale400"
          $style={{
            borderBottom: '1px solid #F1F5F9',
          }}
          overrides={{
            Block: {
              style: {
                ':last-child': {
                  borderBottom: 'none',
                  marginBottom: 0,
                  paddingBottom: 0,
                }
              }
            }
          }}
        >
          <Block marginRight="scale400">
            <Avatar
              name={getTypeIcon(activity.type)}
              size="scale800"
              overrides={{
                Avatar: {
                  style: {
                    backgroundColor: getStatusColor(activity.status),
                    fontSize: '14px',
                  }
                }
              }}
            />
          </Block>
          
          <Block flex="1">
            <ParagraphSmall margin="0" marginBottom="scale100">
              {activity.message}
            </ParagraphSmall>
            <Block display="flex" alignItems="center">
              <LabelSmall color="contentSecondary" marginRight="scale200">
                {activity.timestamp}
              </LabelSmall>
              {activity.user !== 'system' && (
                <LabelSmall color="contentSecondary">
                  by {activity.user}
                </LabelSmall>
              )}
            </Block>
          </Block>
        </Block>
      ))}
    </Block>
  );
}