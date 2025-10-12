'use client';

import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { ChevronLeft } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { NodeForm } from '@/components/nodes/node-form';
import { nodeService, nodeGroupService } from '@/lib/api';
import { Node, NodeGroup } from '@/lib/api/types';

interface EditNodePageProps {
  params: {
    id: string;
  };
}

export default function EditNodePage({ params }: EditNodePageProps) {
  const router = useRouter();
  const [node, setNode] = useState<Node | null>(null);
  const [groups, setGroups] = useState<NodeGroup[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  
  // Load node and groups data
  useEffect(() => {
    const loadData = async () => {
      setIsLoading(true);
      setError(null);
      
      try {
        const [nodeResponse, groupsResponse] = await Promise.all([
          nodeService.getNode(params.id),
          nodeGroupService.getNodeGroups()
        ]);
        
        if (nodeResponse.data) {
          setNode(nodeResponse.data);
        } else {
          setError('Node not found');
        }
        
        if (groupsResponse.data) {
          setGroups(groupsResponse.data);
        }
      } catch (err: any) {
        setError(err.message || 'Failed to load node data');
      } finally {
        setIsLoading(false);
      }
    };
    
    loadData();
  }, [params.id]);
  
  const handleUpdateNode = async (formData: any) => {
    setIsSaving(true);
    setError(null);
    
    try {
      await nodeService.updateNode(params.id, formData);
      router.push('/nodes');
    } catch (err: any) {
      setError(err.message || 'Failed to update node');
      setIsSaving(false);
      throw err;
    }
  };
  
  return (
    <div className="space-y-6">
      <div>
        <Button 
          variant="outline" 
          className="mb-6" 
          onClick={() => router.back()}
        >
          <ChevronLeft className="mr-1 h-4 w-4" />
          Back
        </Button>
        
        <h1 className="text-3xl font-bold tracking-tight">
          Edit Node: {node?.name || 'Loading...'}
        </h1>
        <p className="text-muted-foreground mt-2">
          Update node information.
        </p>
      </div>
      
      {error && (
        <div className="p-4 bg-red-50 text-red-600 rounded-md">
          {error}
        </div>
      )}
      
      {isLoading ? (
        <div className="flex justify-center py-8">
          <div className="animate-spin h-6 w-6 border-2 border-primary border-t-transparent rounded-full"></div>
        </div>
      ) : node ? (
        <NodeForm 
          node={node}
          groups={groups}
          onSubmit={handleUpdateNode}
          isLoading={isSaving}
        />
      ) : null}
    </div>
  );
}
