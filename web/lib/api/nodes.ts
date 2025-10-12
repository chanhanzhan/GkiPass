"use client";

import { ApiClient } from './client';
import { ApiResponse, Node, NodeGroup, PaginationParams } from './types';

/**
 * Node API service
 */
class NodeService {
  /**
   * Get a list of nodes
   */
  async getNodes(params?: {
    type?: string;
    status?: string;
  } & PaginationParams): Promise<ApiResponse<Node[]>> {
    return ApiClient.get<Node[]>('/nodes', params);
  }
  
  /**
   * Get a single node by ID
   */
  async getNode(id: string): Promise<ApiResponse<Node>> {
    return ApiClient.get<Node>(`/nodes/${id}`);
  }
  
  /**
   * Create a new node
   */
  async createNode(node: {
    name: string;
    type: string;
    ip: string;
    port: number;
    group_id?: string;
    description?: string;
  }): Promise<ApiResponse<Node>> {
    return ApiClient.post<Node>('/nodes', node);
  }
  
  /**
   * Update an existing node
   */
  async updateNode(id: string, node: {
    name?: string;
    ip?: string;
    port?: number;
    group_id?: string;
    status?: string;
    description?: string;
  }): Promise<ApiResponse<Node>> {
    return ApiClient.put<Node>(`/nodes/${id}`, node);
  }
  
  /**
   * Delete a node
   */
  async deleteNode(id: string): Promise<ApiResponse<void>> {
    return ApiClient.delete<void>(`/nodes/${id}`);
  }
  
  /**
   * Get node status
   */
  async getNodeStatus(id: string): Promise<ApiResponse<any>> {
    return ApiClient.get<any>(`/nodes/${id}/status`);
  }
  
  /**
   * Send heartbeat for a node
   */
  async sendHeartbeat(id: string, data: {
    load: number;
    connections: number;
  }): Promise<ApiResponse<any>> {
    return ApiClient.post<any>(`/nodes/${id}/heartbeat`, data);
  }
  
  /**
   * Get node's group memberships
   */
  async getNodeGroups(id: string): Promise<ApiResponse<NodeGroup[]>> {
    return ApiClient.get<NodeGroup[]>(`/nodes/${id}/groups`);
  }
  
  /**
   * Generate certificate for node
   */
  async generateCert(nodeId: string): Promise<ApiResponse<any>> {
    return ApiClient.post<any>(`/nodes/${nodeId}/cert`);
  }
  
  /**
   * Download node certificate
   */
  async downloadCert(nodeId: string): Promise<Blob> {
    const response = await fetch(`${ApiClient.baseUrl}/nodes/${nodeId}/cert/download`, {
      headers: {
        Authorization: `Bearer ${localStorage.getItem('gkipass_auth_token')}`,
      },
    });
    
    if (!response.ok) {
      throw new Error('Failed to download certificate');
    }
    
    return response.blob();
  }
}

/**
 * Node Group API service
 */
class NodeGroupService {
  /**
   * Get a list of node groups
   */
  async getNodeGroups(type?: string): Promise<ApiResponse<NodeGroup[]>> {
    return ApiClient.get<NodeGroup[]>('/node-groups', { type });
  }
  
  /**
   * Get a single node group by ID
   */
  async getNodeGroup(id: string): Promise<ApiResponse<NodeGroup>> {
    return ApiClient.get<NodeGroup>(`/node-groups/${id}`);
  }
  
  /**
   * Create a new node group
   */
  async createNodeGroup(group: {
    name: string;
    type: string;
    description?: string;
  }): Promise<ApiResponse<NodeGroup>> {
    return ApiClient.post<NodeGroup>('/node-groups', group);
  }
  
  /**
   * Update an existing node group
   */
  async updateNodeGroup(id: string, group: {
    name?: string;
    type?: string;
    description?: string;
  }): Promise<ApiResponse<NodeGroup>> {
    return ApiClient.put<NodeGroup>(`/node-groups/${id}`, group);
  }
  
  /**
   * Delete a node group
   */
  async deleteNodeGroup(id: string): Promise<ApiResponse<void>> {
    return ApiClient.delete<void>(`/node-groups/${id}`);
  }
  
  /**
   * Get nodes in a group
   */
  async getNodesInGroup(groupId: string): Promise<ApiResponse<Node[]>> {
    return ApiClient.get<Node[]>(`/node-groups/${groupId}/nodes`);
  }
  
  /**
   * Add a node to a group
   */
  async addNodeToGroup(groupId: string, nodeId: string): Promise<ApiResponse<void>> {
    return ApiClient.put<void>(`/node-groups/${groupId}/nodes/${nodeId}`, {});
  }
  
  /**
   * Remove a node from a group
   */
  async removeNodeFromGroup(groupId: string, nodeId: string): Promise<ApiResponse<void>> {
    return ApiClient.delete<void>(`/node-groups/${groupId}/nodes/${nodeId}`);
  }
  
  /**
   * Get all ingress (entry) node groups
   */
  async getIngressGroups(): Promise<ApiResponse<NodeGroup[]>> {
    return ApiClient.get<NodeGroup[]>('/node-groups/ingress');
  }
  
  /**
   * Get all egress (exit) node groups
   */
  async getEgressGroups(): Promise<ApiResponse<NodeGroup[]>> {
    return ApiClient.get<NodeGroup[]>('/node-groups/egress');
  }
}

// Export singleton instances
export const nodeService = new NodeService();
export const nodeGroupService = new NodeGroupService();