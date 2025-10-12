"use client";

import React from 'react';
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer
} from 'recharts';

interface CPUUsageData {
  timestamp: string;
  usage: number;
}

interface CPUUsageChartProps {
  data: CPUUsageData[];
  title?: string;
}

const CPUUsageChart: React.FC<CPUUsageChartProps> = ({ 
  data,
  title = "CPU Usage Over Time"
}) => {
  const formatXAxis = (tickItem: string) => {
    const date = new Date(tickItem);
    return `${date.getHours()}:${String(date.getMinutes()).padStart(2, '0')}`;
  };

  return (
    <div className="w-full p-4 bg-white rounded-lg shadow dark:bg-zinc-800">
      <h3 className="text-lg font-medium mb-4 dark:text-white">{title}</h3>
      <div className="h-64">
        <ResponsiveContainer width="100%" height="100%">
          <LineChart
            data={data}
            margin={{ top: 5, right: 30, left: 20, bottom: 5 }}
          >
            <CartesianGrid strokeDasharray="3 3" stroke="#444" />
            <XAxis 
              dataKey="timestamp" 
              tickFormatter={formatXAxis}
              stroke="#888" 
            />
            <YAxis 
              domain={[0, 100]} 
              unit="%" 
              stroke="#888"
            />
            <Tooltip 
              formatter={(value: number) => [`${value}%`, 'CPU Usage']}
              labelFormatter={(label) => {
                const date = new Date(label);
                return date.toLocaleTimeString();
              }}
            />
            <Legend />
            <Line
              type="monotone"
              dataKey="usage"
              stroke="#2563eb"
              strokeWidth={2}
              activeDot={{ r: 8 }}
              name="CPU Usage"
            />
          </LineChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
};

export default CPUUsageChart;
