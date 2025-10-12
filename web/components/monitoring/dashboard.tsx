"use client";

import React, { useState, useEffect } from 'react';
import { Loader2 } from 'lucide-react';
import StatsOverview from './stats-overview';
import CPUUsageChart from './cpu-usage-chart';
import MemoryUsageChart from './memory-usage-chart';
import NetworkTrafficChart from './network-traffic-chart';
import ConnectionsChart from './connections-chart';
import NodeStatusCard from './node-status-card';
import { MonitoringService } from '@/lib/api/monitoring';

const formatBytes = (bytes: number, decimals = 2) => {
  if (bytes === 0) return '0 Bytes';

  const k = 1024;
  const dm = decimals < 0 ? 0 : decimals;
  const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB', 'PB', 'EB', 'ZB', 'YB'];

  const i = Math.floor(Math.log(bytes) / Math.log(k));

  return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
};

const formatUptime = (seconds: number) => {
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  
  if (days > 0) {
    return `${days}d ${hours}h ${minutes}m`;
  } else if (hours > 0) {
    return `${hours}h ${minutes}m`;
  } else {
    return `${minutes}m`;
  }
};

interface MonitoringDashboardProps {
  nodeId?: string; // If provided, show monitoring for a specific node
}

const MonitoringDashboard: React.FC<MonitoringDashboardProps> = ({ nodeId }) => {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [overviewStats, setOverviewStats] = useState({
    totalNodes: 0,
    onlineNodes: 0,
    activeUsers: 0,
    activeTunnels: 0,
    totalTraffic: "0 B",
    cpuUsage: 0,
    memoryUsage: 0,
    inboundTraffic: "0 B",
    outboundTraffic: "0 B"
  });
  const [nodeStatusList, setNodeStatusList] = useState<any[]>([]);
  const [cpuData, setCpuData] = useState<any[]>([]);
  const [memoryData, setMemoryData] = useState<any[]>([]);
  const [networkData, setNetworkData] = useState<any[]>([]);
  const [connectionsData, setConnectionsData] = useState<any[]>([]);

  useEffect(() => {
    const fetchMonitoringData = async () => {
      try {
        setLoading(true);
        setError(null);
        
        const monitoringService = new MonitoringService();
        
        if (nodeId) {
          // Fetch monitoring data for a specific node
          const nodeData = await monitoringService.getNodeMonitoringData(nodeId);
          if (nodeData.success) {
            const data = nodeData.data;
            
            // Process data for charts
            const timeSeriesData = data.timeSeries || [];
            
            setCpuData(timeSeriesData.map((item: any) => ({
              timestamp: item.timestamp,
              usage: item.cpu_usage
            })));
            
            setMemoryData(timeSeriesData.map((item: any) => ({
              timestamp: item.timestamp,
              usage: item.memory_used,
              total: item.memory_total
            })));
            
            setNetworkData(timeSeriesData.map((item: any) => ({
              timestamp: item.timestamp,
              inbound: item.traffic_in_bytes,
              outbound: item.traffic_out_bytes
            })));
            
            setConnectionsData(timeSeriesData.map((item: any) => ({
              timestamp: item.timestamp,
              connections: item.total_connections,
              tunnels: item.active_tunnels
            })));
            
            // Set overview stats
            const latest = timeSeriesData[timeSeriesData.length - 1] || {};
            setOverviewStats({
              totalNodes: 1,
              onlineNodes: data.isOnline ? 1 : 0,
              activeUsers: 0,
              activeTunnels: latest.active_tunnels || 0,
              totalTraffic: formatBytes(
                (latest.traffic_in_bytes || 0) + (latest.traffic_out_bytes || 0)
              ),
              cpuUsage: latest.cpu_usage || 0,
              memoryUsage: latest.memory_usage_percent || 0,
              inboundTraffic: formatBytes(latest.traffic_in_bytes || 0),
              outboundTraffic: formatBytes(latest.traffic_out_bytes || 0)
            });
          }
        } else {
          // Fetch global monitoring overview
          const overview = await monitoringService.getMonitoringOverview();
          if (overview.success) {
            const data = overview.data;
            
            setOverviewStats({
              totalNodes: data.totalNodes || 0,
              onlineNodes: data.onlineNodes || 0,
              activeUsers: data.activeUsers || 0,
              activeTunnels: data.activeTunnels || 0,
              totalTraffic: formatBytes(data.totalTrafficBytes || 0),
              cpuUsage: data.avgCpuUsage || 0,
              memoryUsage: data.avgMemoryUsage || 0,
              inboundTraffic: formatBytes(data.inboundTrafficBytes || 0),
              outboundTraffic: formatBytes(data.outboundTrafficBytes || 0),
            });
            
            // Process data for charts
            const timeSeriesData = data.timeSeriesData || [];
            
            setCpuData(timeSeriesData.map((item: any) => ({
              timestamp: item.timestamp,
              usage: item.avgCpuUsage
            })));
            
            setMemoryData(timeSeriesData.map((item: any) => ({
              timestamp: item.timestamp,
              usage: item.memoryUsed,
              total: item.memoryTotal
            })));
            
            setNetworkData(timeSeriesData.map((item: any) => ({
              timestamp: item.timestamp,
              inbound: item.inboundTraffic,
              outbound: item.outboundTraffic
            })));
            
            setConnectionsData(timeSeriesData.map((item: any) => ({
              timestamp: item.timestamp,
              connections: item.totalConnections,
              tunnels: item.activeTunnels
            })));
            
            // Fetch all node statuses
            const nodeStatuses = await monitoringService.getNodesStatus();
            if (nodeStatuses.success) {
              setNodeStatusList(nodeStatuses.data);
            }
          }
        }
      } catch (err) {
        console.error("Error fetching monitoring data:", err);
        setError("Failed to fetch monitoring data");
      } finally {
        setLoading(false);
      }
    };

    fetchMonitoringData();
    
    // Set up polling for real-time updates
    const intervalId = setInterval(fetchMonitoringData, 30000); // Update every 30 seconds
    
    return () => clearInterval(intervalId);
  }, [nodeId]);

  if (loading) {
    return (
      <div className="flex justify-center items-center min-h-[400px]">
        <Loader2 className="h-8 w-8 animate-spin text-blue-600" />
        <span className="ml-2 text-lg">Loading monitoring data...</span>
      </div>
    );
  }

  if (error) {
    return (
      <div className="bg-red-50 dark:bg-red-900/20 border-l-4 border-red-500 p-4 my-4">
        <div className="flex">
          <div className="flex-shrink-0">
            <svg className="h-5 w-5 text-red-500" viewBox="0 0 20 20" fill="currentColor">
              <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
            </svg>
          </div>
          <div className="ml-3">
            <p className="text-sm font-medium text-red-800 dark:text-red-200">
              {error}
            </p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <StatsOverview stats={overviewStats} />
      
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <CPUUsageChart data={cpuData} />
        <MemoryUsageChart data={memoryData} />
      </div>
      
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <NetworkTrafficChart data={networkData} />
        <ConnectionsChart data={connectionsData} />
      </div>
      
      {!nodeId && nodeStatusList.length > 0 && (
        <div>
          <h2 className="text-xl font-medium mb-4 dark:text-white">Node Status</h2>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {nodeStatusList.map((node) => (
              <NodeStatusCard
                key={node.id}
                id={node.id}
                name={node.name}
                status={node.status}
                cpuUsage={node.cpuUsage}
                memoryUsage={node.memoryUsage}
                uptime={node.uptime ? formatUptime(node.uptime) : undefined}
                lastSeen={node.lastSeen ? new Date(node.lastSeen).toLocaleString() : undefined}
                connections={node.connections}
                onClick={() => window.location.href = `/dashboard/nodes/${node.id}/monitoring`}
              />
            ))}
          </div>
        </div>
      )}
    </div>
  );
};

export default MonitoringDashboard;
