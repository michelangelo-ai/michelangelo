import React from 'react';
import { Block } from 'baseui/block';
import { ParagraphSmall } from 'baseui/typography';

interface MetricsChartProps {
  type: 'requests' | 'latency' | 'errors' | 'resources';
  timeRange: string;
  deployment: string;
}

export function MetricsChart({ type, timeRange, deployment }: MetricsChartProps) {
  // Mock data generator for demonstration
  const generateMockData = () => {
    const points = 20;
    const data = [];
    const now = new Date();
    
    for (let i = points - 1; i >= 0; i--) {
      const time = new Date(now.getTime() - i * 60 * 60 * 1000); // Hourly intervals
      let value = 0;
      
      switch (type) {
        case 'requests':
          value = Math.random() * 100 + 50; // 50-150 requests/min
          break;
        case 'latency':
          value = Math.random() * 50 + 100; // 100-150ms
          break;
        case 'errors':
          value = Math.random() * 5; // 0-5% error rate
          break;
        case 'resources':
          value = Math.random() * 40 + 30; // 30-70% usage
          break;
      }
      
      data.push({ time: time.getHours(), value });
    }
    
    return data;
  };

  const data = generateMockData();
  const maxValue = Math.max(...data.map(d => d.value));
  const minValue = Math.min(...data.map(d => d.value));

  const getMetricInfo = () => {
    switch (type) {
      case 'requests':
        return { unit: 'req/min', color: '#1B7CFC', currentValue: Math.round(data[data.length - 1].value) };
      case 'latency':
        return { unit: 'ms', color: '#F59E0B', currentValue: Math.round(data[data.length - 1].value) };
      case 'errors':
        return { unit: '%', color: '#EF4444', currentValue: data[data.length - 1].value.toFixed(2) };
      case 'resources':
        return { unit: '%', color: '#8B5CF6', currentValue: Math.round(data[data.length - 1].value) };
      default:
        return { unit: '', color: '#6B7280', currentValue: 0 };
    }
  };

  const { unit, color, currentValue } = getMetricInfo();

  return (
    <Block>
      {/* Current Value Display */}
      <Block display="flex" justifyContent="space-between" alignItems="center" marginBottom="scale400">
        <ParagraphSmall margin="0" color="contentSecondary">
          Current: <span style={{ color, fontWeight: 600 }}>{currentValue} {unit}</span>
        </ParagraphSmall>
        <ParagraphSmall margin="0" color="contentSecondary">
          Range: {minValue.toFixed(1)} - {maxValue.toFixed(1)} {unit}
        </ParagraphSmall>
      </Block>

      {/* Simple Line Chart Visualization */}
      <Block
        height="200px"
        backgroundColor="#F8FAFC"
        borderRadius="8px"
        padding="scale400"
        position="relative"
        overflow="hidden"
      >
        {/* Chart Grid Lines */}
        {[0, 25, 50, 75, 100].map((percent) => (
          <Block
            key={percent}
            position="absolute"
            left="scale400"
            right="scale400"
            top={`${percent}%`}
            height="1px"
            backgroundColor={percent === 0 ? '#E2E8F0' : '#F1F5F9'}
          />
        ))}

        {/* Data Points and Line */}
        <svg
          width="100%"
          height="100%"
          style={{ position: 'absolute', top: 0, left: 0 }}
        >
          {/* Line Path */}
          <polyline
            points={data.map((point, index) => {
              const x = (index / (data.length - 1)) * 100;
              const y = ((maxValue - point.value) / (maxValue - minValue)) * 80 + 10;
              return `${x}%,${y}%`;
            }).join(' ')}
            fill="none"
            stroke={color}
            strokeWidth="2"
            style={{ vectorEffect: 'non-scaling-stroke' }}
          />
          
          {/* Data Points */}
          {data.map((point, index) => {
            const x = (index / (data.length - 1)) * 100;
            const y = ((maxValue - point.value) / (maxValue - minValue)) * 80 + 10;
            return (
              <circle
                key={index}
                cx={`${x}%`}
                cy={`${y}%`}
                r="3"
                fill={color}
                style={{ vectorEffect: 'non-scaling-stroke' }}
              />
            );
          })}
        </svg>

        {/* X-axis labels */}
        <Block
          position="absolute"
          bottom="0"
          left="scale400"
          right="scale400"
          display="flex"
          justifyContent="space-between"
        >
          {data.filter((_, index) => index % 4 === 0).map((point, index) => (
            <ParagraphSmall key={index} margin="0" color="contentSecondary" fontSize="10px">
              {point.time}:00
            </ParagraphSmall>
          ))}
        </Block>
      </Block>

      {/* Trend Information */}
      <Block marginTop="scale400">
        <ParagraphSmall margin="0" color="contentSecondary">
          {deployment === 'all' ? 'All deployments' : deployment} • {timeRange}
        </ParagraphSmall>
      </Block>
    </Block>
  );
}