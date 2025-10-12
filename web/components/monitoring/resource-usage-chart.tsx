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
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

interface ResourceUsageChartProps {
  data: Array<{
    timestamp: string;
    cpu: number;
    memory: number;
    connections: number;
  }>;
  title?: string;
  height?: number;
  showConnections?: boolean;
}

const ResourceUsageChart: React.FC<ResourceUsageChartProps> = ({ 
  data, 
  title = "Resource Usage", 
  height = 400,
  showConnections = true
}) => {
  // Format the timestamp for display
  const formattedData = data.map(item => ({
    ...item,
    time: new Date(item.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  }));

  return (
    <Card className="w-full">
      <CardHeader>
        <CardTitle>{title}</CardTitle>
      </CardHeader>
      <CardContent>
        <div style={{ width: '100%', height }}>
          <ResponsiveContainer>
            <AreaChart
              data={formattedData}
              margin={{ top: 10, right: 30, left: 0, bottom: 0 }}
            >
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="time" />
              <YAxis 
                yAxisId="left"
                tickFormatter={(value) => `${value}%`}
              />
              {showConnections && (
                <YAxis 
                  yAxisId="right" 
                  orientation="right" 
                  domain={[0, 'auto']} 
                />
              )}
              <Tooltip 
                formatter={(value: number, name: string) => {
                  if (name === 'cpu' || name === 'memory') {
                    return [`${value}%`, name.toUpperCase()];
                  }
                  return [value, 'Connections'];
                }}
              />
              <Legend />
              <Area 
                yAxisId="left"
                type="monotone" 
                dataKey="cpu" 
                name="CPU" 
                stroke="#ef4444" 
                fill="#fee2e2" 
                strokeWidth={2}
              />
              <Area 
                yAxisId="left"
                type="monotone" 
                dataKey="memory" 
                name="Memory" 
                stroke="#8b5cf6" 
                fill="#ede9fe" 
                strokeWidth={2}
              />
              {showConnections && (
                <Area 
                  yAxisId="right"
                  type="monotone" 
                  dataKey="connections" 
                  name="Connections" 
                  stroke="#0ea5e9" 
                  fill="#e0f2fe" 
                  strokeWidth={2}
                />
              )}
            </AreaChart>
          </ResponsiveContainer>
        </div>
      </CardContent>
    </Card>
  );
};

export default ResourceUsageChart;
