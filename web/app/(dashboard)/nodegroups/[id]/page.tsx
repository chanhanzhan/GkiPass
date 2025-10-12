'use client';

import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { ChevronLeft, Edit, Settings, Server, Plus } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { NodesTable } from '@/components/nodes/nodes-table';
import { nodeService, nodeGroupService } from '@/lib/api';
import { Node, NodeGroup } from '@/lib/api/types';
import Link from 'next/link';

interface NodeGroupPageProps {
  params: {
    id: string;
  };
}

export default function NodeGroupPage({ params }: NodeGroupPageProps) {
  const router = useRouter();
  const [group, setGroup] = useState<NodeGroup | null>(null);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  
  // Load node group and its nodes
  useEffect(() => {
    const loadData = async () => {
      setIsLoading(true);
      setError(null);
      
      try {
        const [groupResponse, nodesResponse] = await Promise.all([
          nodeGroupService.getNodeGroup(params.id),
          nodeGroupService.getNodesInGroup(params.id)
        ]);
        
        if (groupResponse.data) {
          setGroup(groupResponse.data);
        } else {
          setError('Node group not found');
        }
        
        if (nodesResponse.data) {
          setNodes(nodesResponse.data);
        }
      } catch (err: any) {
        setError(err.message || 'Failed to load node group data');
      } finally {
        setIsLoading(false);
      }
    };
    
    loadData();
  }, [params.id]);

  const handleDeleteNode = async (nodeId: string) => {
    if (!confirm('Are you sure you want to remove this node from the group?')) return;
    
    try {
      await nodeGroupService.removeNodeFromGroup(params.id, nodeId);
      setNodes(nodes.filter((node) => node.id !== nodeId));
    } catch (err: any) {
      setError(err.message || 'Failed to remove node from group');
    }
  };

  const getGroupTypeBadge = () => {
    if (!group) return null;
    
    switch (group.type) {
      case 'entry':
        return <span className="bg-blue-100 text-blue-800 px-2 py-1 rounded-full text-xs font-medium">Entry</span>;
      case 'exit':
        return <span className="bg-green-100 text-green-800 px-2 py-1 rounded-full text-xs font-medium">Exit</span>;
      default:
        return null;
    }
  };
  
  return (
    <div className="space-y-6">
      <div>
        <Button 
          variant="outline" 
          className="mb-6" 
          onClick={() => router.push('/nodegroups')}
        >
          <ChevronLeft className="mr-1 h-4 w-4" />
          Back to Groups
        </Button>
        
        {isLoading ? (
          <h1 className="text-3xl font-bold tracking-tight">Loading...</h1>
        ) : group ? (
          <div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
            <div>
              <div className="flex items-center gap-2">
                <h1 className="text-3xl font-bold tracking-tight">{group.name}</h1>
                {getGroupTypeBadge()}
              </div>
              <p className="text-muted-foreground mt-2">
                {group.description || `A ${group.type} node group with ${group.nodeCount} nodes.`}
              </p>
            </div>
            
            <div className="flex items-center gap-2">
              <Button variant="outline" size="sm" asChild>
                <Link href={`/nodegroups/${params.id}/edit`}>
                  <Edit className="mr-1 h-4 w-4" />
                  Edit
                </Link>
              </Button>
              <Button variant="outline" size="sm" asChild>
                <Link href={`/nodegroups/${params.id}/config`}>
                  <Settings className="mr-1 h-4 w-4" />
                  Config
                </Link>
              </Button>
            </div>
          </div>
        ) : null}
      </div>
      
      {error && (
        <div className="p-4 bg-red-50 text-red-600 rounded-md">
          {error}
        </div>
      )}
      
      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle>Nodes in this Group</CardTitle>
          <Button size="sm" asChild>
            <Link href={`/nodegroups/${params.id}/add-nodes`}>
              <Plus className="mr-1 h-4 w-4" />
              Add Nodes
            </Link>
          </Button>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <div className="flex justify-center py-8">
              <div className="animate-spin h-6 w-6 border-2 border-primary border-t-transparent rounded-full"></div>
            </div>
          ) : nodes.length > 0 ? (
            <NodesTable 
              nodes={nodes}
              onDeleteNode={handleDeleteNode}
            />
          ) : (
            <div className="py-12 text-center">
              <Server className="mx-auto h-12 w-12 text-muted-foreground/50" />
              <h3 className="mt-4 text-lg font-medium">No nodes in this group</h3>
              <p className="mt-2 text-muted-foreground">
                Add nodes to this group to manage them together.
              </p>
              <Button className="mt-4" size="sm" asChild>
                <Link href={`/nodegroups/${params.id}/add-nodes`}>
                  <Plus className="mr-1 h-4 w-4" />
                  Add Nodes
                </Link>
              </Button>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
