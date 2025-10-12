'use client';

import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { ChevronLeft } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { NodeForm } from '@/components/nodes/node-form';
import { nodeService, nodeGroupService } from '@/lib/api';
import { NodeGroup } from '@/lib/api/types';

export default function CreateNodePage() {
  const router = useRouter();
  const [groups, setGroups] = useState<NodeGroup[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  
  // Load node groups for the form
  useEffect(() => {
    const loadGroups = async () => {
      try {
        const response = await nodeGroupService.getNodeGroups();
        if (response.data) {
          setGroups(response.data);
        }
      } catch (err: any) {
        setError(err.message || 'Failed to load node groups');
      }
    };
    
    loadGroups();
  }, []);
  
  const handleCreateNode = async (formData: any) => {
    setIsLoading(true);
    setError(null);
    
    try {
      await nodeService.createNode(formData);
      router.push('/nodes');
    } catch (err: any) {
      setError(err.message || 'Failed to create node');
      setIsLoading(false);
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
        
        <h1 className="text-3xl font-bold tracking-tight">Create Node</h1>
        <p className="text-muted-foreground mt-2">
          Add a new node to your network.
        </p>
      </div>
      
      {error && (
        <div className="p-4 bg-red-50 text-red-600 rounded-md">
          {error}
        </div>
      )}
      
      <NodeForm 
        groups={groups}
        onSubmit={handleCreateNode}
        isLoading={isLoading}
      />
    </div>
  );
}
