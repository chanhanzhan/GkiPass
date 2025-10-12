'use client';

import { useState, useEffect } from 'react';
import Link from 'next/link';
import { Plus, RefreshCw, Filter } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { TunnelsTable } from '@/components/tunnels/tunnels-table';
import { Input } from '@/components/ui/input';
import { tunnelService } from '@/lib/api';
import { Tunnel } from '@/lib/api/types';

export default function TunnelsPage() {
  const [tunnels, setTunnels] = useState<Tunnel[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  
  // Filters
  const [protocolFilter, setProtocolFilter] = useState<string>('');
  const [statusFilter, setStatusFilter] = useState<string>('');
  const [searchQuery, setSearchQuery] = useState('');

  const loadData = async () => {
    setIsLoading(true);
    setError(null);
    
    try {
      const enabledParam = statusFilter === 'active' ? true : statusFilter === 'disabled' ? false : undefined;
      
      const response = await tunnelService.getTunnels(enabledParam);
      
      if (response.data) {
        setTunnels(response.data);
      }
    } catch (err: any) {
      setError(err.message || 'Failed to load tunnels');
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    loadData();
  }, [statusFilter]);

  const handleDeleteTunnel = async (tunnelId: string) => {
    if (!confirm('Are you sure you want to delete this tunnel?')) return;
    
    try {
      await tunnelService.deleteTunnel(tunnelId);
      setTunnels(tunnels.filter((tunnel) => tunnel.id !== tunnelId));
    } catch (err: any) {
      setError(err.message || 'Failed to delete tunnel');
    }
  };

  const handleToggleTunnel = async (tunnelId: string, enabled: boolean) => {
    try {
      await tunnelService.toggleTunnel(tunnelId, enabled);
      
      // Update local state
      setTunnels(tunnels.map((tunnel) => 
        tunnel.id === tunnelId ? { ...tunnel, enabled } : tunnel
      ));
    } catch (err: any) {
      setError(err.message || `Failed to ${enabled ? 'enable' : 'disable'} tunnel`);
    }
  };

  // Filter tunnels
  let filteredTunnels = [...tunnels];
  
  if (protocolFilter) {
    filteredTunnels = filteredTunnels.filter(tunnel => tunnel.protocol === protocolFilter);
  }
  
  if (searchQuery) {
    filteredTunnels = filteredTunnels.filter(tunnel => 
      tunnel.name.toLowerCase().includes(searchQuery.toLowerCase())
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Tunnels</h1>
          <p className="text-muted-foreground">
            Manage your network tunnels.
          </p>
        </div>
        
        <div className="flex items-center gap-2">
          <Button asChild>
            <Link href="/tunnels/create">
              <Plus className="mr-1 h-4 w-4" />
              Create Tunnel
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
            <label className="text-sm font-medium">Filter by Protocol</label>
            <select
              className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
              value={protocolFilter}
              onChange={(e) => setProtocolFilter(e.target.value)}
            >
              <option value="">All Protocols</option>
              <option value="tcp">TCP</option>
              <option value="udp">UDP</option>
              <option value="http">HTTP</option>
              <option value="https">HTTPS</option>
            </select>
          </div>
          
          <div className="space-y-2">
            <label className="text-sm font-medium">Filter by Status</label>
            <select
              className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
              value={statusFilter}
              onChange={(e) => setStatusFilter(e.target.value)}
            >
              <option value="">All Status</option>
              <option value="active">Active</option>
              <option value="disabled">Disabled</option>
            </select>
          </div>
          
          <div className="p-4 bg-muted/40 rounded-md">
            <h3 className="font-medium text-sm mb-2">Quick Stats</h3>
            <dl className="space-y-1 text-sm">
              <div className="flex justify-between">
                <dt>Total Tunnels:</dt>
                <dd className="font-medium">{tunnels.length}</dd>
              </div>
              <div className="flex justify-between">
                <dt>Active:</dt>
                <dd className="font-medium">{tunnels.filter(t => t.enabled).length}</dd>
              </div>
              <div className="flex justify-between">
                <dt>Disabled:</dt>
                <dd className="font-medium">{tunnels.filter(t => !t.enabled).length}</dd>
              </div>
            </dl>
          </div>
        </div>
        
        <div className="md:col-span-3 space-y-4">
          <div>
            <div className="relative">
              <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
              <Input
                className="pl-8"
                placeholder="Search tunnels..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
              />
            </div>
          </div>
          
          <TunnelsTable 
            tunnels={filteredTunnels} 
            onDeleteTunnel={handleDeleteTunnel}
            onToggleTunnel={handleToggleTunnel}
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
