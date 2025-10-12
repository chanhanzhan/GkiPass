"use client";

import { useState } from 'react';
import { nodeService, useApiQuery } from '@/lib/api';
import { Node } from '@/lib/api/types';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { 
  AlertCircle, 
  Check, 
  Loader2, 
  RefreshCw, 
  Server, 
  Trash2
} from 'lucide-react';

export function NodeListExample() {
  const [nodeType, setNodeType] = useState<string | undefined>(undefined);
  
  // Use API query hook to fetch nodes with automatic loading state
  const { 
    data: nodes, 
    isLoading, 
    error, 
    refetch,
    isRefetching
  } = useApiQuery<Node[]>(
    () => nodeService.getNodes({ type: nodeType }),
    [nodeType] // Re-fetch when nodeType changes
  );
  
  // Function to get status badge based on node status
  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'online':
        return <Badge variant="success" className="bg-green-500"><Check className="mr-1 h-3 w-3" /> Online</Badge>;
      case 'offline':
        return <Badge variant="secondary" className="bg-gray-400"><AlertCircle className="mr-1 h-3 w-3" /> Offline</Badge>;
      case 'error':
        return <Badge variant="destructive"><AlertCircle className="mr-1 h-3 w-3" /> Error</Badge>;
      default:
        return <Badge>{status}</Badge>;
    }
  };
  
  // Render loading state
  if (isLoading) {
    return (
      <div className="flex justify-center items-center h-64">
        <Loader2 className="h-8 w-8 animate-spin text-gray-400" />
      </div>
    );
  }
  
  // Render error state
  if (error) {
    return (
      <div className="p-4 border border-red-200 rounded-md bg-red-50 text-red-700 flex flex-col items-center">
        <AlertCircle className="h-8 w-8 mb-2" />
        <p className="text-center">Failed to load nodes: {error}</p>
        <Button 
          variant="outline" 
          className="mt-4"
          onClick={() => refetch()}
          disabled={isRefetching}
        >
          {isRefetching && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
          Try Again
        </Button>
      </div>
    );
  }
  
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">Nodes</h2>
        <div className="flex items-center gap-2">
          <Button
            variant={nodeType === undefined ? "default" : "outline"}
            onClick={() => setNodeType(undefined)}
            size="sm"
          >
            All
          </Button>
          <Button
            variant={nodeType === 'client' ? "default" : "outline"}
            onClick={() => setNodeType('client')}
            size="sm"
          >
            Clients
          </Button>
          <Button
            variant={nodeType === 'server' ? "default" : "outline"}
            onClick={() => setNodeType('server')}
            size="sm"
          >
            Servers
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => refetch()}
            disabled={isRefetching}
          >
            {isRefetching ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <RefreshCw className="h-4 w-4" />
            )}
          </Button>
        </div>
      </div>
      
      {nodes && nodes.length > 0 ? (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Type</TableHead>
              <TableHead>IP</TableHead>
              <TableHead>Port</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Last Seen</TableHead>
              <TableHead>Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {nodes.map((node) => (
              <TableRow key={node.id}>
                <TableCell className="font-medium">{node.name}</TableCell>
                <TableCell>
                  <Badge variant="outline">
                    <Server className="mr-1 h-3 w-3" />
                    {node.type}
                  </Badge>
                </TableCell>
                <TableCell>{node.ip}</TableCell>
                <TableCell>{node.port}</TableCell>
                <TableCell>{getStatusBadge(node.status)}</TableCell>
                <TableCell>
                  {new Date(node.last_seen).toLocaleString()}
                </TableCell>
                <TableCell>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-8 w-8 p-0 text-red-500"
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      ) : (
        <div className="text-center p-8 border border-dashed rounded-lg">
          <Server className="mx-auto h-8 w-8 text-gray-400" />
          <p className="mt-2 text-sm text-gray-500">No nodes found</p>
          <Button
            variant="outline"
            className="mt-4"
            size="sm"
          >
            Add Node
          </Button>
        </div>
      )}
    </div>
  );
}
