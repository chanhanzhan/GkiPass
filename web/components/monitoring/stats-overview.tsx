"use client";

import React from 'react';
import { 
  Activity, 
  Server, 
  Users, 
  Workflow, 
  ArrowUp, 
  ArrowDown, 
  Cpu, 
  MemoryStick 
} from 'lucide-react';
import { cn } from '@/lib/utils';

interface StatCardProps {
  title: string;
  value: string | number;
  icon: React.ReactNode;
  trend?: {
    value: number;
    isPositive: boolean;
    label: string;
  };
  className?: string;
}

const StatCard: React.FC<StatCardProps> = ({ title, value, icon, trend, className }) => (
  <div className={cn("bg-white dark:bg-zinc-800 p-4 rounded-lg shadow", className)}>
    <div className="flex justify-between">
      <div>
        <p className="text-sm text-gray-500 dark:text-gray-400 mb-1">{title}</p>
        <p className="text-2xl font-medium dark:text-white">{value}</p>
        {trend && (
          <div className="flex items-center mt-2 text-xs">
            {trend.isPositive ? (
              <ArrowUp size={12} className="text-green-500 mr-1" />
            ) : (
              <ArrowDown size={12} className="text-red-500 mr-1" />
            )}
            <span 
              className={trend.isPositive ? 'text-green-500' : 'text-red-500'}
            >
              {trend.value}% {trend.label}
            </span>
          </div>
        )}
      </div>
      <div className="p-2 rounded-lg bg-blue-100 dark:bg-blue-900">
        {icon}
      </div>
    </div>
  </div>
);

interface StatsOverviewProps {
  stats: {
    totalNodes: number;
    onlineNodes: number;
    activeUsers: number;
    activeTunnels: number;
    totalTraffic: string;
    cpuUsage: number;
    memoryUsage: number;
    inboundTraffic: string;
    outboundTraffic: string;
    trends?: {
      nodes?: { value: number; isPositive: boolean };
      users?: { value: number; isPositive: boolean };
      tunnels?: { value: number; isPositive: boolean };
      traffic?: { value: number; isPositive: boolean };
    };
  };
}

const StatsOverview: React.FC<StatsOverviewProps> = ({ stats }) => {
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
      <StatCard 
        title="Total Nodes" 
        value={`${stats.onlineNodes}/${stats.totalNodes}`} 
        icon={<Server size={24} className="text-blue-600 dark:text-blue-400" />}
        trend={stats.trends?.nodes && {
          value: stats.trends.nodes.value,
          isPositive: stats.trends.nodes.isPositive,
          label: "from last week"
        }}
      />
      
      <StatCard 
        title="Active Users" 
        value={stats.activeUsers} 
        icon={<Users size={24} className="text-indigo-600 dark:text-indigo-400" />}
        trend={stats.trends?.users && {
          value: stats.trends.users.value,
          isPositive: stats.trends.users.isPositive,
          label: "from last week"
        }}
      />
      
      <StatCard 
        title="Active Tunnels" 
        value={stats.activeTunnels} 
        icon={<Workflow size={24} className="text-green-600 dark:text-green-400" />}
        trend={stats.trends?.tunnels && {
          value: stats.trends.tunnels.value,
          isPositive: stats.trends.tunnels.isPositive,
          label: "from last week"
        }}
      />
      
      <StatCard 
        title="Total Traffic" 
        value={stats.totalTraffic} 
        icon={<Activity size={24} className="text-purple-600 dark:text-purple-400" />}
        trend={stats.trends?.traffic && {
          value: stats.trends.traffic.value,
          isPositive: stats.trends.traffic.isPositive,
          label: "from last week"
        }}
      />

      <StatCard 
        title="CPU Usage" 
        value={`${stats.cpuUsage}%`} 
        icon={<Cpu size={24} className="text-yellow-600 dark:text-yellow-400" />}
      />
      
      <StatCard 
        title="Memory Usage" 
        value={`${stats.memoryUsage}%`} 
        icon={<MemoryStick size={24} className="text-red-600 dark:text-red-400" />}
      />
      
      <StatCard 
        title="Inbound Traffic" 
        value={stats.inboundTraffic} 
        icon={<ArrowDown size={24} className="text-teal-600 dark:text-teal-400" />}
      />
      
      <StatCard 
        title="Outbound Traffic" 
        value={stats.outboundTraffic} 
        icon={<ArrowUp size={24} className="text-orange-600 dark:text-orange-400" />}
      />
    </div>
  );
};

export default StatsOverview;
