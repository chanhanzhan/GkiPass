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

interface MemoryUsageData {
  timestamp: string;
  usage: number;
  total: number;
}

interface MemoryUsageChartProps {
  data: MemoryUsageData[];
  title?: string;
}

const MemoryUsageChart: React.FC<MemoryUsageChartProps> = ({ 
  data,
  title = "Memory Usage Over Time"
}) => {
  const formatXAxis = (tickItem: string) => {
    const date = new Date(tickItem);
    return `${date.getHours()}:${String(date.getMinutes()).padStart(2, '0')}`;
  };

  const formatBytes = (bytes: number, decimals = 2) => {
    if (bytes === 0) return '0 Bytes';

    const k = 1024;
    const dm = decimals < 0 ? 0 : decimals;
    const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB', 'PB', 'EB', 'ZB', 'YB'];

    const i = Math.floor(Math.log(bytes) / Math.log(k));

    return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
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
              stroke="#888"
              tickFormatter={(value) => formatBytes(value, 0)}
            />
            <Tooltip 
              formatter={(value: number) => [formatBytes(value), 'Memory']}
              labelFormatter={(label) => {
                const date = new Date(label);
                return date.toLocaleTimeString();
              }}
            />
            <Legend />
            <Line
              type="monotone"
              dataKey="usage"
              stroke="#10b981"
              strokeWidth={2}
              activeDot={{ r: 8 }}
              name="Used Memory"
            />
            <Line
              type="monotone"
              dataKey="total"
              stroke="#6b7280"
              strokeDasharray="5 5"
              strokeWidth={1}
              name="Total Memory"
            />
          </LineChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
};

export default MemoryUsageChart;
