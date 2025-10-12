"use client";

import React from 'react';
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer
} from 'recharts';
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

interface LatencyChartProps {
  data: Array<{
    name: string;
    min: number;
    avg: number;
    max: number;
  }>;
  title?: string;
  height?: number;
}

const LatencyChart: React.FC<LatencyChartProps> = ({ 
  data, 
  title = "Latency by Region", 
  height = 400 
}) => {
  return (
    <Card className="w-full">
      <CardHeader>
        <CardTitle>{title}</CardTitle>
      </CardHeader>
      <CardContent>
        <div style={{ width: '100%', height }}>
          <ResponsiveContainer>
            <BarChart
              data={data}
              margin={{ top: 5, right: 30, left: 20, bottom: 5 }}
            >
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="name" />
              <YAxis 
                tickFormatter={(value) => `${value}ms`}
              />
              <Tooltip 
                formatter={(value: number) => [`${value}ms`, '']}
              />
              <Legend />
              <Bar dataKey="min" name="Min Latency" fill="#10b981" />
              <Bar dataKey="avg" name="Avg Latency" fill="#3b82f6" />
              <Bar dataKey="max" name="Max Latency" fill="#ef4444" />
            </BarChart>
          </ResponsiveContainer>
        </div>
      </CardContent>
    </Card>
  );
};

export default LatencyChart;
