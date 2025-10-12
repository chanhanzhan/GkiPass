'use client';

import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { ChevronLeft } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { TunnelForm } from '@/components/tunnels/tunnel-form';
import { tunnelService, nodeGroupService } from '@/lib/api';
import { Tunnel, NodeGroup } from '@/lib/api/types';

interface EditTunnelPageProps {
  params: {
    id: string;
  };
}

export default function EditTunnelPage({ params }: EditTunnelPageProps) {
  const router = useRouter();
  const [tunnel, setTunnel] = useState<Tunnel | null>(null);
  const [entryGroups, setEntryGroups] = useState<NodeGroup[]>([]);
  const [exitGroups, setExitGroups] = useState<NodeGroup[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  
  // Load tunnel and node groups
  useEffect(() => {
    const loadData = async () => {
      setIsLoading(true);
      setError(null);
      
      try {
        const [tunnelResponse, groupsResponse] = await Promise.all([
          tunnelService.getTunnel(params.id),
          nodeGroupService.getNodeGroups()
        ]);
        
        if (tunnelResponse.data) {
          setTunnel(tunnelResponse.data);
        } else {
          setError('Tunnel not found');
        }
        
        if (groupsResponse.data) {
          const groups = groupsResponse.data;
          setEntryGroups(groups.filter((g) => g.type === 'entry'));
          setExitGroups(groups.filter((g) => g.type === 'exit'));
        }
      } catch (err: any) {
        setError(err.message || 'Failed to load data');
      } finally {
        setIsLoading(false);
      }
    };
    
    loadData();
  }, [params.id]);
  
  const handleUpdateTunnel = async (formData: any) => {
    setIsSaving(true);
    setError(null);
    
    try {
      await tunnelService.updateTunnel(params.id, formData);
      router.push('/tunnels');
    } catch (err: any) {
      setError(err.message || 'Failed to update tunnel');
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
          Edit Tunnel: {tunnel?.name || 'Loading...'}
        </h1>
        <p className="text-muted-foreground mt-2">
          Update tunnel configuration.
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
      ) : tunnel ? (
        <TunnelForm 
          tunnel={tunnel}
          entryGroups={entryGroups}
          exitGroups={exitGroups}
          onSubmit={handleUpdateTunnel}
          isLoading={isSaving}
        />
      ) : null}
    </div>
  );
}
