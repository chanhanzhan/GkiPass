'use client';

import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { ChevronLeft } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { NodeGroupForm } from '@/components/nodegroups/nodegroup-form';
import { nodeGroupService } from '@/lib/api';
import { NodeGroup } from '@/lib/api/types';

interface EditNodeGroupPageProps {
  params: {
    id: string;
  };
}

export default function EditNodeGroupPage({ params }: EditNodeGroupPageProps) {
  const router = useRouter();
  const [group, setGroup] = useState<NodeGroup | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  
  // Load node group data
  useEffect(() => {
    const loadData = async () => {
      setIsLoading(true);
      setError(null);
      
      try {
        const response = await nodeGroupService.getNodeGroup(params.id);
        
        if (response.data) {
          setGroup(response.data);
        } else {
          setError('Node group not found');
        }
      } catch (err: any) {
        setError(err.message || 'Failed to load node group data');
      } finally {
        setIsLoading(false);
      }
    };
    
    loadData();
  }, [params.id]);
  
  const handleUpdateNodeGroup = async (formData: any) => {
    setIsSaving(true);
    setError(null);
    
    try {
      await nodeGroupService.updateNodeGroup(params.id, formData);
      router.push('/nodegroups');
    } catch (err: any) {
      setError(err.message || 'Failed to update node group');
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
          Edit Node Group: {group?.name || 'Loading...'}
        </h1>
        <p className="text-muted-foreground mt-2">
          Update node group information.
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
      ) : group ? (
        <NodeGroupForm 
          group={group}
          onSubmit={handleUpdateNodeGroup}
          isLoading={isSaving}
        />
      ) : null}
    </div>
  );
}
