'use client';

import { useState, useEffect } from 'react';
import Link from 'next/link';
import { Plus, RefreshCw } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { NodeGroupsTable } from '@/components/nodegroups/nodegroups-table';
import { Input } from '@/components/ui/input';
import { nodeGroupService } from '@/lib/api';
import { NodeGroup } from '@/lib/api/types';

export default function NodeGroupsPage() {
  const [groups, setGroups] = useState<NodeGroup[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState('');
  
  const loadData = async () => {
    setIsLoading(true);
    setError(null);
    
    try {
      const response = await nodeGroupService.getNodeGroups();
      
      if (response.data) {
        setGroups(response.data);
      }
    } catch (err: any) {
      setError(err.message || 'Failed to load node groups');
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    loadData();
  }, []);

  const handleDeleteGroup = async (groupId: string) => {
    if (!confirm('Are you sure you want to delete this node group?')) return;
    
    try {
      await nodeGroupService.deleteNodeGroup(groupId);
      setGroups(groups.filter((group) => group.id !== groupId));
    } catch (err: any) {
      setError(err.message || 'Failed to delete node group');
    }
  };

  // Filter groups by search query
  const filteredGroups = searchQuery
    ? groups.filter((group) => 
        group.name.toLowerCase().includes(searchQuery.toLowerCase())
      )
    : groups;

  return (
    <div className="space-y-6">
      <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Node Groups</h1>
          <p className="text-muted-foreground">
            Manage your node groups for easier tunnel management.
          </p>
        </div>
        
        <div className="flex items-center gap-2">
          <Button asChild>
            <Link href="/nodegroups/create">
              <Plus className="mr-1 h-4 w-4" />
              New Group
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
      
      <div className="space-y-4">
        <div>
          <div className="relative">
            <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
            <Input
              className="pl-8"
              placeholder="Search node groups..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
            />
          </div>
        </div>
        
        <NodeGroupsTable 
          groups={filteredGroups} 
          onDeleteGroup={handleDeleteGroup} 
        />
        
        {isLoading && (
          <div className="flex justify-center py-8">
            <div className="animate-spin h-6 w-6 border-2 border-primary border-t-transparent rounded-full"></div>
          </div>
        )}
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
