"use client";

import { ApiClient } from './client';
import { ApiResponse, Tunnel, TunnelTarget } from './types';

/**
 * Tunnels API service
 */
class TunnelService {
  /**
   * Get a list of tunnels
   */
  async getTunnels(params?: {
    user_id?: string;
    enabled?: boolean;
  }): Promise<ApiResponse<Tunnel[]>> {
    return ApiClient.get<Tunnel[]>('/tunnels', params);
  }
  
  /**
   * Get a single tunnel by ID
   */
  async getTunnel(id: string): Promise<ApiResponse<Tunnel>> {
    return ApiClient.get<Tunnel>(`/tunnels/${id}`);
  }
  
  /**
   * Create a new tunnel
   */
  async createTunnel(tunnel: {
    name: string;
    protocol: string;
    entry_group_id: string;
    exit_group_id: string;
    local_port: number;
    targets: TunnelTarget[];
    enabled?: boolean;
    description?: string;
  }): Promise<ApiResponse<Tunnel>> {
    // Convert targets array to JSON string format expected by API
    const tunnelData = {
      ...tunnel,
      targets: JSON.stringify(tunnel.targets)
    };
    
    return ApiClient.post<Tunnel>('/tunnels', tunnelData);
  }
  
  /**
   * Update an existing tunnel
   */
  async updateTunnel(id: string, tunnel: {
    name?: string;
    protocol?: string;
    entry_group_id?: string;
    exit_group_id?: string;
    local_port?: number;
    targets?: TunnelTarget[];
    enabled?: boolean;
    description?: string;
  }): Promise<ApiResponse<Tunnel>> {
    // Convert targets array to JSON string if present
    const tunnelData = { ...tunnel };
    if (tunnelData.targets) {
      tunnelData.targets = JSON.stringify(tunnelData.targets);
    }
    
    return ApiClient.put<Tunnel>(`/tunnels/${id}`, tunnelData);
  }
  
  /**
   * Delete a tunnel
   */
  async deleteTunnel(id: string): Promise<ApiResponse<void>> {
    return ApiClient.delete<void>(`/tunnels/${id}`);
  }
  
  /**
   * Enable a tunnel
   */
  async enableTunnel(id: string): Promise<ApiResponse<Tunnel>> {
    return ApiClient.put<Tunnel>(`/tunnels/${id}`, { enabled: true });
  }
  
  /**
   * Disable a tunnel
   */
  async disableTunnel(id: string): Promise<ApiResponse<Tunnel>> {
    return ApiClient.put<Tunnel>(`/tunnels/${id}`, { enabled: false });
  }
  
  /**
   * Probe a tunnel connection
   */
  async probeTunnel(id: string): Promise<ApiResponse<any>> {
    return ApiClient.post<any>(`/tunnels/${id}/probe`, {});
  }
  
  /**
   * Get traffic statistics for a tunnel
   */
  async getTunnelStats(id: string, params?: {
    start_date?: string;
    end_date?: string;
  }): Promise<ApiResponse<any>> {
    return ApiClient.get<any>(`/tunnels/${id}/stats`, params);
  }
  
  /**
   * Parse targets string from API response to array of TunnelTarget objects
   */
  parseTunnelTargets(targetsString: string): TunnelTarget[] {
    try {
      return JSON.parse(targetsString);
    } catch (e) {
      console.error('Failed to parse tunnel targets', e);
      return [];
    }
  }
}

// Export a singleton instance
export const tunnelService = new TunnelService();