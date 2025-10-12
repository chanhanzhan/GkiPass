'use client';

import { useState, useEffect } from 'react';
import Link from 'next/link';
import { Plus, Filter, RefreshCw, Server } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { NodesTable } from '@/components/nodes/nodes-table';
import { Input } from '@/components/ui/input';
import { nodeService, nodeGroupService } from '@/lib/api';
import { Node, NodeGroup } from '@/lib/api/types';

export default function NodesPage() {
  const [nodes, setNodes] = useState<Node[]>([]);
  const [nodeGroups, setNodeGroups] = useState<NodeGroup[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  
  // Filters
  const [typeFilter, setTypeFilter] = useState<string>('');
  const [statusFilter, setStatusFilter] = useState<string>('');
  const [searchQuery, setSearchQuery] = useState('');

  const loadData = async () => {
    setIsLoading(true);
    setError(null);
    
    try {
      // Build filter params
      const params: any = {};
      if (typeFilter) params.type = typeFilter;
      if (statusFilter) params.status = statusFilter;
      
      const [nodesResponse, groupsResponse] = await Promise.all([
        nodeService.getNodes(params),
        nodeGroupService.getNodeGroups()
      ]);
      
      if (nodesResponse.data) {
        setNodes(nodesResponse.data);
      }
      
      if (groupsResponse.data) {
        setNodeGroups(groupsResponse.data);
      }
    } catch (err: any) {
      setError(err.message || 'Failed to load nodes');
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    loadData();
  }, [typeFilter, statusFilter]);

  const handleDeleteNode = async (nodeId: string) => {
    if (!confirm('Are you sure you want to delete this node?')) return;
    
    try {
      await nodeService.deleteNode(nodeId);
      setNodes(nodes.filter((node) => node.id !== nodeId));
    } catch (err: any) {
      setError(err.message || 'Failed to delete node');
    }
  };

  // Filter nodes by search query
  const filteredNodes = searchQuery
    ? nodes.filter((node) => 
        node.name.toLowerCase().includes(searchQuery.toLowerCase()) || 
        node.ip.includes(searchQuery)
      )
    : nodes;

  return (
    <div className="space-y-6">
      <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Nodes</h1>
          <p className="text-muted-foreground">
            Manage your network nodes.
          </p>
        </div>
        
        <div className="flex items-center gap-2">
          <Button asChild>
            <Link href="/nodes/create">
              <Plus className="mr-1 h-4 w-4" />
              Add Node
            </Link>
          </Button>
          <Button variant="outline" onClick={loadData}>
            <RefreshCw className="mr-1 h-4 w-4" />
            Refresh
          </Button>
        </div>
      </div>
      
      {error && (
        <div className="p-4 bg-red-50 text-red-600 rounded-md">
          {error}
        </div>
      )}
      
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
        <div className="space-y-4">
          <div className="space-y-2">
            <label className="text-sm font-medium">Filter by Type</label>
            <select
              className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
              value={typeFilter}
              onChange={(e) => setTypeFilter(e.target.value)}
            >
              <option value="">All Types</option>
              <option value="client">Client</option>
              <option value="server">Server</option>
            </select>
          </div>
          
          <div className="space-y-2">
            <label className="text-sm font-medium">Filter by Status</label>
            <select
              className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
              value={statusFilter}
              onChange={(e) => setStatusFilter(e.target.value)}
            >
              <option value="">All Statuses</option>
              <option value="online">Online</option>
              <option value="offline">Offline</option>
              <option value="error">Error</option>
            </select>
          </div>
          
          <div className="space-y-2">
            <label className="text-sm font-medium">Node Groups</label>
            <div className="flex flex-col space-y-2">
              {nodeGroups.length === 0 ? (
                <p className="text-sm text-muted-foreground">No node groups found.</p>
              ) : (
                nodeGroups.map((group) => (
                  <Link
                    key={group.id}
                    href={`/nodegroups/${group.id}`}
                    className="flex items-center p-2 text-sm hover:bg-muted rounded-md"
                  >
                    <Server className="h-4 w-4 mr-2" />
                    {group.name} ({group.nodeCount})
                  </Link>
                ))
              )}
            </div>
          </div>
        </div>
        
        <div className="md:col-span-3 space-y-4">
          <div>
            <div className="relative">
              <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
              <Input
                className="pl-8"
                placeholder="Search by name or IP..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
              />
            </div>
          </div>
          
          <NodesTable 
            nodes={filteredNodes} 
            onDeleteNode={handleDeleteNode} 
          />
          
          {isLoading && (
            <div className="flex justify-center py-8">
              <div className="animate-spin h-6 w-6 border-2 border-primary border-t-transparent rounded-full"></div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function Search(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg
      {...props}
      xmlns="http://www.w3.org/2000/svg"
      width="24"
      height="24"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <circle cx="11" cy="11" r="8" />
      <path d="m21 21-4.3-4.3" />
    </svg>
  );
}
