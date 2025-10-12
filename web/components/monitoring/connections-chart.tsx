"use client";

import React from 'react';
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer
} from 'recharts';

interface ConnectionsData {
  timestamp: string;
  connections: number;
  tunnels: number;
}

interface ConnectionsChartProps {
  data: ConnectionsData[];
  title?: string;
}

const ConnectionsChart: React.FC<ConnectionsChartProps> = ({ 
  data,
  title = "Active Connections" 
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
          <AreaChart
            data={data}
            margin={{ top: 5, right: 30, left: 20, bottom: 5 }}
          >
            <CartesianGrid strokeDasharray="3 3" stroke="#444" />
            <XAxis 
              dataKey="timestamp" 
              tickFormatter={formatXAxis}
              stroke="#888" 
            />
            <YAxis stroke="#888" />
            <Tooltip 
              labelFormatter={(label) => {
                const date = new Date(label);
                return date.toLocaleTimeString();
              }}
            />
            <Legend />
            <Area
              type="monotone"
              dataKey="connections"
              stroke="#8b5cf6"
              fill="#8b5cf6"
              fillOpacity={0.3}
              name="Connections"
            />
            <Area
              type="monotone"
              dataKey="tunnels"
              stroke="#f59e0b"
              fill="#f59e0b"
              fillOpacity={0.3}
              name="Active Tunnels"
            />
          </AreaChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
};

export default ConnectionsChart;
