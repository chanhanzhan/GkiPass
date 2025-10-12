"use client";

import { ApiClient } from './client';
import { ApiResponse, NodeMonitoringData } from './types';

/**
 * Monitoring API service
 */
class MonitoringService {
  /**
   * Get node monitoring data
   */
  async getNodeMonitoringData(nodeId: string, params?: {
    from?: string;
    to?: string;
    limit?: number;
  }): Promise<ApiResponse<NodeMonitoringData[]>> {
    return ApiClient.get<NodeMonitoringData[]>(`/monitoring/nodes/${nodeId}/data`, params);
  }
  
  /**
   * Get node monitoring status
   */
  async getNodeMonitoringStatus(nodeId: string): Promise<ApiResponse<any>> {
    return ApiClient.get<any>(`/monitoring/nodes/${nodeId}/status`);
  }
  
  /**
   * Get node performance history
   */
  async getNodePerformanceHistory(nodeId: string, params?: {
    type?: string; // hourly, daily, weekly
    from?: string;
    to?: string;
  }): Promise<ApiResponse<any>> {
    return ApiClient.get<any>(`/monitoring/nodes/${nodeId}/history`, params);
  }
  
  /**
   * Get monitoring configuration for a node
   */
  async getNodeMonitoringConfig(nodeId: string): Promise<ApiResponse<any>> {
    return ApiClient.get<any>(`/monitoring/nodes/${nodeId}/config`);
  }
  
  /**
   * Update monitoring configuration for a node
   */
  async updateNodeMonitoringConfig(nodeId: string, config: {
    monitoring_enabled?: boolean;
    report_interval?: number;
    collect_system_info?: boolean;
    collect_network_stats?: boolean;
    collect_tunnel_stats?: boolean;
    collect_performance?: boolean;
    data_retention_days?: number;
    alert_cpu_threshold?: number;
    alert_memory_threshold?: number;
    alert_disk_threshold?: number;
  }): Promise<ApiResponse<any>> {
    return ApiClient.put<any>(`/monitoring/nodes/${nodeId}/config`, config);
  }
  
  /**
   * Get node alerts
   */
  async getNodeAlerts(nodeId: string, params?: {
    status?: string; // active, resolved, all
    limit?: number;
  }): Promise<ApiResponse<any>> {
    return ApiClient.get<any>(`/monitoring/nodes/${nodeId}/alerts`, params);
  }
  
  /**
   * Get system monitoring summary
   */
  async getSystemMonitoringSummary(): Promise<ApiResponse<any>> {
    return ApiClient.get<any>('/monitoring/system');
  }
  
  /**
   * Get rules monitoring data
   */
  async getRulesMonitoring(): Promise<ApiResponse<any>> {
    return ApiClient.get<any>('/monitoring/rules');
  }
  
  /**
   * Report monitoring data (for nodes)
   */
  async reportMonitoringData(nodeId: string, data: any): Promise<ApiResponse<void>> {
    return ApiClient.post<void>(`/monitoring/nodes/${nodeId}/report`, data);
  }
  
  /**
   * Get monitoring overview for all nodes
   */
  async getNodeMonitoringOverview(): Promise<ApiResponse<any>> {
    return ApiClient.get<any>('/monitoring/nodes/overview');
  }
  
  /**
   * Create monitoring permission for a user
   */
  async createMonitoringPermission(data: {
    user_id: string;
    node_id: string;
    permission_type: string;
    enabled?: boolean;
    description?: string;
  }): Promise<ApiResponse<any>> {
    return ApiClient.post<any>('/monitoring/permissions', data);
  }
  
  /**
   * Get monitoring permissions for current user
   */
  async getMyMonitoringPermissions(): Promise<ApiResponse<any>> {
    return ApiClient.get<any>('/monitoring/permissions/me');
  }
}

// Export a singleton instance
export const monitoringService = new MonitoringService();