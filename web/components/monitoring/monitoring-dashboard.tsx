"use client";

import React, { useState, useEffect } from 'react';
import { monitoringService, nodeService } from '@/lib/api';
import { useApiError } from '@/lib/hooks/use-api-error';
import { toast } from '@/lib/ui/use-toast';
import StatsOverview from './stats-overview';
import TrafficChart from './traffic-chart';
import ResourceUsageChart from './resource-usage-chart';
import LatencyChart from './latency-chart';
import NodeStatusChart from './node-status-chart';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Loader2, AlertTriangle } from 'lucide-react';

// Sample data for demo purposes
const sampleTrafficData = Array.from({ length: 24 }, (_, i) => ({
  timestamp: new Date(Date.now() - (23 - i) * 3600000).toISOString(),
  inbound: Math.floor(Math.random() * 1000000000),
  outbound: Math.floor(Math.random() * 800000000),
}));

const sampleResourceData = Array.from({ length: 24 }, (_, i) => ({
  timestamp: new Date(Date.now() - (23 - i) * 3600000).toISOString(),
  cpu: Math.floor(Math.random() * 80) + 10,
  memory: Math.floor(Math.random() * 70) + 20,
  connections: Math.floor(Math.random() * 200) + 50,
}));

const sampleLatencyData = [
  { name: 'US East', min: 15, avg: 45, max: 120 },
  { name: 'US West', min: 35, avg: 85, max: 180 },
  { name: 'Europe', min: 80, avg: 150, max: 250 },
  { name: 'Asia', min: 120, avg: 220, max: 350 },
  { name: 'Australia', min: 150, avg: 280, max: 420 },
];

const sampleNodeStatusData = [
  { name: 'Online', value: 42 },
  { name: 'Degraded', value: 8 },
  { name: 'Offline', value: 5 },
  { name: 'Maintenance', value: 3 },
];

interface MonitoringDashboardProps {
  nodeId?: string; // Optional - if provided, shows monitoring for a specific node
}

const MonitoringDashboard: React.FC<MonitoringDashboardProps> = ({ nodeId }) => {
  const { isLoading, error, handleError, withErrorHandling } = useApiError();
  const [timeRange, setTimeRange] = useState('24h');
  const [isFirstLoad, setIsFirstLoad] = useState(true);
  const [stats, setStats] = useState({
    totalNodes: 58,
    onlineNodes: 42,
    activeUsers: 156,
    activeTunnels: 24,
    totalTraffic: '3.8 TB',
    cpuUsage: 42,
    memoryUsage: 58,
    inboundTraffic: '2.1 TB',
    outboundTraffic: '1.7 TB',
    trends: {
      nodes: { value: 5, isPositive: true },
      users: { value: 12, isPositive: true },
      tunnels: { value: 8, isPositive: true },
      traffic: { value: 15, isPositive: true },
    },
  });
  
  const [trafficData, setTrafficData] = useState(sampleTrafficData);
  const [resourceData, setResourceData] = useState(sampleResourceData);
  const [latencyData, setLatencyData] = useState(sampleLatencyData);
  const [nodeStatusData, setNodeStatusData] = useState(sampleNodeStatusData);

  // Function to fetch and process node data
  const fetchNodeData = async () => {
    try {
      // If nodeId is provided, fetch specific node data
      if (nodeId) {
        // Fetch node details first to show node name
        const nodeResponse = await withErrorHandling(
          async () => await nodeService.getNode(nodeId)
        );
        
        if (nodeResponse?.data) {
          toast({
            title: "Node Selected",
            description: `Viewing monitoring data for ${nodeResponse.data.name || nodeId}`,
          });
        }
        
        // Fetch monitoring data
        const response = await withErrorHandling(
          async () => await monitoringService.getNodeMonitoringData(nodeId, {
            from: getTimeRangeDate(timeRange),
            to: new Date().toISOString(),
          })
        );
        
        if (response?.data) {
          // Process the data for charts
          // In a real implementation, we would transform the API data
          // For now, we'll continue using sample data
          console.log('Node monitoring data:', response.data);
          
          // Simulate processing the data
          setTimeout(() => {
            // This would normally be transformed from the API response
            setResourceData(sampleResourceData.map(item => ({
              ...item,
              // Add some variation based on nodeId
              cpu: Math.min(100, item.cpu + (nodeId.charCodeAt(0) % 10)),
              memory: Math.min(100, item.memory + (nodeId.charCodeAt(1) % 15))
            })));
          }, 300);
        }
      } else {
        // Fetch overview data
        const response = await withErrorHandling(
          async () => await monitoringService.getMonitoringOverview()
        );
        
        if (response?.data) {
          // Process the data for charts
          console.log('Monitoring overview:', response.data);
          
          // In a real implementation, we would transform the API data
          // For now, let's simulate some real-time data
          
          // Get node counts by status
          const nodeStatusResponse = await withErrorHandling(
            async () => await nodeService.getNodes()
          );
          
          if (nodeStatusResponse?.data) {
            // Calculate node status distribution
            const statusCounts = {
              Online: 0,
              Degraded: 0,
              Offline: 0,
              Maintenance: 0
            };
            
            nodeStatusResponse.data.forEach(node => {
              if (node.status === 'online') statusCounts.Online++;
              else if (node.status === 'degraded') statusCounts.Degraded++;
              else if (node.status === 'offline') statusCounts.Offline++;
              else if (node.status === 'maintenance') statusCounts.Maintenance++;
            });
            
            // Update node status chart
            setNodeStatusData([
              { name: 'Online', value: statusCounts.Online || 42 },
              { name: 'Degraded', value: statusCounts.Degraded || 8 },
              { name: 'Offline', value: statusCounts.Offline || 5 },
              { name: 'Maintenance', value: statusCounts.Maintenance || 3 },
            ]);
            
            // Update stats
            setStats(prev => ({
              ...prev,
              totalNodes: nodeStatusResponse.data.length,
              onlineNodes: statusCounts.Online
            }));
          }
        }
      }
    } catch (error) {
      // Error is already handled by withErrorHandling
      console.error('Error in fetchNodeData:', error);
    } finally {
      setIsFirstLoad(false);
    }
  };
  
  // Load monitoring data
  useEffect(() => {
    fetchNodeData();
  }, [nodeId, timeRange]);

  // Helper to get date for time range
  const getTimeRangeDate = (range: string): string => {
    const now = new Date();
    
    switch (range) {
      case '1h':
        return new Date(now.getTime() - 3600000).toISOString();
      case '6h':
        return new Date(now.getTime() - 6 * 3600000).toISOString();
      case '24h':
        return new Date(now.getTime() - 24 * 3600000).toISOString();
      case '7d':
        return new Date(now.getTime() - 7 * 24 * 3600000).toISOString();
      case '30d':
        return new Date(now.getTime() - 30 * 24 * 3600000).toISOString();
      default:
        return new Date(now.getTime() - 24 * 3600000).toISOString();
    }
  };

  if (isLoading && isFirstLoad) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
        <span className="ml-2">Loading monitoring data...</span>
      </div>
    );
  }

  if (error && isFirstLoad) {
    return (
      <div className="flex items-center justify-center h-64 text-red-500">
        <AlertTriangle className="h-8 w-8 mr-2" />
        <span>{error}</span>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Stats Overview */}
      <StatsOverview stats={stats} />
      
      {/* Time Range Selector */}
      <div className="flex justify-between items-center">
        <div>
          {isLoading && (
            <div className="flex items-center text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin mr-2" />
              Refreshing data...
            </div>
          )}
        </div>
        <Select
          value={timeRange}
          onValueChange={setTimeRange}
        >
          <SelectTrigger className="w-[180px]">
            <SelectValue placeholder="Select time range" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="1h">Last Hour</SelectItem>
            <SelectItem value="6h">Last 6 Hours</SelectItem>
            <SelectItem value="24h">Last 24 Hours</SelectItem>
            <SelectItem value="7d">Last 7 Days</SelectItem>
            <SelectItem value="30d">Last 30 Days</SelectItem>
          </SelectContent>
        </Select>
      </div>
      
      {/* Main Charts */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <TrafficChart 
          data={trafficData} 
          title="Network Traffic" 
        />
        
        <ResourceUsageChart 
          data={resourceData} 
          title="Resource Utilization" 
        />
      </div>
      
      {/* Additional Charts */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="lg:col-span-2">
          <LatencyChart 
            data={latencyData} 
            title="Network Latency by Region" 
          />
        </div>
        
        <NodeStatusChart 
          data={nodeStatusData} 
          title="Node Status Distribution" 
        />
      </div>
      
      {/* Detailed Metrics Tabs */}
      <Card>
        <CardHeader>
          <CardTitle>Detailed Metrics</CardTitle>
        </CardHeader>
        <CardContent>
          <Tabs defaultValue="traffic">
            <TabsList className="grid w-full grid-cols-4">
              <TabsTrigger value="traffic">Traffic</TabsTrigger>
              <TabsTrigger value="performance">Performance</TabsTrigger>
              <TabsTrigger value="errors">Errors</TabsTrigger>
              <TabsTrigger value="alerts">Alerts</TabsTrigger>
            </TabsList>
            <TabsContent value="traffic" className="pt-4">
              <div className="h-[300px] flex items-center justify-center bg-muted/20 rounded-md">
                <p className="text-muted-foreground">Detailed traffic metrics will be displayed here</p>
              </div>
            </TabsContent>
            <TabsContent value="performance" className="pt-4">
              <div className="h-[300px] flex items-center justify-center bg-muted/20 rounded-md">
                <p className="text-muted-foreground">Performance metrics will be displayed here</p>
              </div>
            </TabsContent>
            <TabsContent value="errors" className="pt-4">
              <div className="h-[300px] flex items-center justify-center bg-muted/20 rounded-md">
                <p className="text-muted-foreground">Error logs and statistics will be displayed here</p>
              </div>
            </TabsContent>
            <TabsContent value="alerts" className="pt-4">
              <div className="h-[300px] flex items-center justify-center bg-muted/20 rounded-md">
                <p className="text-muted-foreground">System alerts will be displayed here</p>
              </div>
            </TabsContent>
          </Tabs>
        </CardContent>
      </Card>
    </div>
  );
};

export default MonitoringDashboard;
