'use client';

import { useState, useEffect } from 'react';
import { useParams, useRouter } from 'next/navigation';
import { Button } from '@/components/ui/button';
import { ArrowLeft, Loader2 } from 'lucide-react';
import MonitoringDashboard from '@/components/monitoring/monitoring-dashboard';
import { nodeService } from '@/lib/api';
import { Node } from '@/lib/api/types';

export default function NodeMonitoringPage() {
  const params = useParams();
  const router = useRouter();
  const nodeId = params.id as string;
  
  const [isLoading, setIsLoading] = useState(true);
  const [node, setNode] = useState<Node | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const fetchNode = async () => {
      setIsLoading(true);
      setError(null);
      
      try {
        const response = await nodeService.getNode(nodeId);
        if (response.data) {
          setNode(response.data);
        }
      } catch (err: any) {
        setError(err.message || 'Failed to load node details');
        console.error('Error loading node:', err);
      } finally {
        setIsLoading(false);
      }
    };
    
    fetchNode();
  }, [nodeId]);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
        <span className="ml-2">Loading node details...</span>
      </div>
    );
  }

  if (error) {
    return (
      <div className="space-y-6">
        <Button 
          variant="outline" 
          onClick={() => router.back()}
          className="flex items-center gap-2"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to Monitoring
        </Button>
        
        <div className="flex items-center justify-center h-64 text-red-500">
          <span>{error}</span>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Button 
          variant="outline" 
          onClick={() => router.back()}
          className="flex items-center gap-2"
        >
          <ArrowLeft className="h-4 w-4" />
          Back
        </Button>
        
        <div>
          <h1 className="text-3xl font-bold tracking-tight">
            Node Monitoring: {node?.name || nodeId}
          </h1>
          <p className="text-muted-foreground">
            {node?.description || 'Detailed monitoring for this node'}
          </p>
        </div>
      </div>
      
      <MonitoringDashboard nodeId={nodeId} />
    </div>
  );
}