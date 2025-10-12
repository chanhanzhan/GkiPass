'use client';

import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { ChevronLeft } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { TunnelForm } from '@/components/tunnels/tunnel-form';
import { tunnelService, nodeGroupService } from '@/lib/api';
import { NodeGroup } from '@/lib/api/types';

export default function CreateTunnelPage() {
  const router = useRouter();
  const [entryGroups, setEntryGroups] = useState<NodeGroup[]>([]);
  const [exitGroups, setExitGroups] = useState<NodeGroup[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  
  // Load node groups
  useEffect(() => {
    const loadGroups = async () => {
      setIsLoading(true);
      setError(null);
      
      try {
        // In a real app, we would use the proper endpoints to get entry and exit groups
        // Here we're simulating by loading all groups and filtering by type
        const response = await nodeGroupService.getNodeGroups();
        
        if (response.data) {
          const groups = response.data;
          setEntryGroups(groups.filter((g) => g.type === 'entry'));
          setExitGroups(groups.filter((g) => g.type === 'exit'));
        }
      } catch (err: any) {
        setError(err.message || 'Failed to load node groups');
      } finally {
        setIsLoading(false);
      }
    };
    
    loadGroups();
  }, []);
  
  const handleCreateTunnel = async (formData: any) => {
    setIsSaving(true);
    setError(null);
    
    try {
      await tunnelService.createTunnel(formData);
      router.push('/tunnels');
    } catch (err: any) {
      setError(err.message || 'Failed to create tunnel');
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
        
        <h1 className="text-3xl font-bold tracking-tight">Create Tunnel</h1>
        <p className="text-muted-foreground mt-2">
          Set up a new network tunnel.
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
      ) : (
        <TunnelForm 
          entryGroups={entryGroups}
          exitGroups={exitGroups}
          onSubmit={handleCreateTunnel}
          isLoading={isSaving}
        />
      )}
    </div>
  );
}
