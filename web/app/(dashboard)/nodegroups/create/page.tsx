'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { ChevronLeft } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { NodeGroupForm } from '@/components/nodegroups/nodegroup-form';
import { nodeGroupService } from '@/lib/api';

export default function CreateNodeGroupPage() {
  const router = useRouter();
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  
  const handleCreateNodeGroup = async (formData: any) => {
    setIsLoading(true);
    setError(null);
    
    try {
      await nodeGroupService.createNodeGroup(formData);
      router.push('/nodegroups');
    } catch (err: any) {
      setError(err.message || 'Failed to create node group');
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
        
        <h1 className="text-3xl font-bold tracking-tight">Create Node Group</h1>
        <p className="text-muted-foreground mt-2">
          Create a new group to organize your nodes.
        </p>
      </div>
      
      {error && (
        <div className="p-4 bg-red-50 text-red-600 rounded-md">
          {error}
        </div>
      )}
      
      <NodeGroupForm 
        onSubmit={handleCreateNodeGroup}
        isLoading={isLoading}
      />
    </div>
  );
}
