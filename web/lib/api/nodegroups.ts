import apiClient from './client';
import { ApiResponse, NodeGroup, Node } from './types';

export class NodeGroupsService {
  // Get all node groups
  async getNodeGroups(type?: string): Promise<ApiResponse<NodeGroup[]>> {
    let endpoint = '/api/nodegroups';
    
    if (type) {
      endpoint += `?type=${type}`;
    }
    
    return apiClient.get<NodeGroup[]>(endpoint);
  }
  
  // Get single node group
  async getNodeGroup(id: string): Promise<ApiResponse<NodeGroup>> {
    return apiClient.get<NodeGroup>(`/api/nodegroups/${id}`);
  }
  
  // Create new node group
  async createNodeGroup(nodeGroup: Partial<NodeGroup>): Promise<ApiResponse<NodeGroup>> {
    return apiClient.post<NodeGroup>('/api/nodegroups', nodeGroup);
  }
  
  // Update node group
  async updateNodeGroup(id: string, nodeGroup: Partial<NodeGroup>): Promise<ApiResponse<NodeGroup>> {
    return apiClient.put<NodeGroup>(`/api/nodegroups/${id}`, nodeGroup);
  }
  
  // Delete node group
  async deleteNodeGroup(id: string): Promise<ApiResponse<void>> {
    return apiClient.delete<void>(`/api/nodegroups/${id}`);
  }
  
  // Get nodes in group
  async getNodesInGroup(groupId: string): Promise<ApiResponse<Node[]>> {
    return apiClient.get<Node[]>(`/api/nodegroups/${groupId}/nodes`);
  }
  
  // Add node to group
  async addNodeToGroup(groupId: string, nodeId: string): Promise<ApiResponse<void>> {
    return apiClient.post<void>(`/api/nodegroups/${groupId}/nodes`, { nodeId });
  }
  
  // Remove node from group
  async removeNodeFromGroup(groupId: string, nodeId: string): Promise<ApiResponse<void>> {
    return apiClient.delete<void>(`/api/nodegroups/${groupId}/nodes/${nodeId}`);
  }
  
  // Get node group configuration
  async getNodeGroupConfig(groupId: string): Promise<ApiResponse<any>> {
    return apiClient.get<any>(`/api/nodegroups/${groupId}/config`);
  }
  
  // Update node group configuration
  async updateNodeGroupConfig(groupId: string, config: any): Promise<ApiResponse<any>> {
    return apiClient.put<any>(`/api/nodegroups/${groupId}/config`, config);
  }

  // Get ingress (entry) groups
  async getIngressGroups(): Promise<ApiResponse<NodeGroup[]>> {
    return apiClient.get<NodeGroup[]>('/api/nodegroups/ingress');
  }
  
  // Get egress (exit) groups
  async getEgressGroups(): Promise<ApiResponse<NodeGroup[]>> {
    return apiClient.get<NodeGroup[]>('/api/nodegroups/egress');
  }
}

// Create a singleton instance
export const nodeGroupService = new NodeGroupsService();

// Export the singleton instance as default
export default nodeGroupService;
