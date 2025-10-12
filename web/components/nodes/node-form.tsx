'use client';

import { useState, useEffect } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from '@/components/ui/card';
import { Node, NodeGroup } from '@/lib/api/types';

// Form validation schema
const nodeSchema = z.object({
  name: z.string().min(1, 'Name is required').max(100),
  type: z.enum(['client', 'server']),
  ip: z.string().min(1, 'IP address is required'),
  port: z.coerce.number().int().min(1, 'Port is required').max(65535),
  groupId: z.string().optional(),
  description: z.string().optional(),
});

type NodeFormValues = z.infer<typeof nodeSchema>;

interface NodeFormProps {
  node?: Node;
  groups?: NodeGroup[];
  onSubmit: (data: NodeFormValues) => Promise<void>;
  isLoading?: boolean;
}

export function NodeForm({ node, groups, onSubmit, isLoading }: NodeFormProps) {
  const [error, setError] = useState<string | null>(null);

  // Initialize form
  const {
    register,
    handleSubmit,
    formState: { errors },
    reset,
  } = useForm<NodeFormValues>({
    resolver: zodResolver(nodeSchema),
    defaultValues: {
      name: '',
      type: 'client',
      ip: '',
      port: 0,
      groupId: '',
      description: '',
    },
  });

  // Set form defaults when node prop changes
  useEffect(() => {
    if (node) {
      reset({
        name: node.name,
        type: node.type as 'client' | 'server',
        ip: node.ip,
        port: node.port,
        groupId: node.groupId || '',
        description: node.description || '',
      });
    }
  }, [node, reset]);

  const handleFormSubmit = async (data: NodeFormValues) => {
    setError(null);
    try {
      await onSubmit(data);
    } catch (err: any) {
      setError(err.message || 'An error occurred while saving the node');
    }
  };

  const isEdit = !!node;

  return (
    <Card className="w-full">
      <CardHeader>
        <CardTitle>{isEdit ? 'Edit Node' : 'Add New Node'}</CardTitle>
      </CardHeader>
      <form onSubmit={handleSubmit(handleFormSubmit)}>
        <CardContent className="space-y-4">
          {error && (
            <div className="p-3 text-sm bg-red-50 text-red-600 rounded-md">{error}</div>
          )}
          
          <div className="space-y-2">
            <label htmlFor="name" className="text-sm font-medium">
              Node Name
            </label>
            <Input
              id="name"
              placeholder="Enter node name"
              {...register('name')}
              disabled={isLoading}
            />
            {errors.name && (
              <p className="text-sm text-red-500">{errors.name.message}</p>
            )}
          </div>
          
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div className="space-y-2">
              <label htmlFor="type" className="text-sm font-medium">
                Node Type
              </label>
              <select
                id="type"
                className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                {...register('type')}
                disabled={isLoading || isEdit}
              >
                <option value="client">Client</option>
                <option value="server">Server</option>
              </select>
              {errors.type && (
                <p className="text-sm text-red-500">{errors.type.message}</p>
              )}
            </div>
            
            <div className="space-y-2">
              <label htmlFor="groupId" className="text-sm font-medium">
                Node Group
              </label>
              <select
                id="groupId"
                className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                {...register('groupId')}
                disabled={isLoading}
              >
                <option value="">Select a group (optional)</option>
                {groups?.map((group) => (
                  <option key={group.id} value={group.id}>
                    {group.name} ({group.type})
                  </option>
                ))}
              </select>
            </div>
          </div>
          
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div className="space-y-2">
              <label htmlFor="ip" className="text-sm font-medium">
                IP Address
              </label>
              <Input
                id="ip"
                placeholder="Enter IP address"
                {...register('ip')}
                disabled={isLoading}
              />
              {errors.ip && (
                <p className="text-sm text-red-500">{errors.ip.message}</p>
              )}
            </div>
            
            <div className="space-y-2">
              <label htmlFor="port" className="text-sm font-medium">
                Port
              </label>
              <Input
                id="port"
                type="number"
                placeholder="Enter port"
                {...register('port')}
                disabled={isLoading}
              />
              {errors.port && (
                <p className="text-sm text-red-500">{errors.port.message}</p>
              )}
            </div>
          </div>
          
          <div className="space-y-2">
            <label htmlFor="description" className="text-sm font-medium">
              Description
            </label>
            <textarea
              id="description"
              className="flex min-h-24 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
              placeholder="Optional description"
              {...register('description')}
              disabled={isLoading}
            />
          </div>
        </CardContent>
        
        <CardFooter className="flex justify-end space-x-2">
          <Button 
            type="button" 
            variant="outline"
            disabled={isLoading}
            onClick={() => reset()}
          >
            Reset
          </Button>
          <Button 
            type="submit" 
            disabled={isLoading}
          >
            {isLoading ? 'Saving...' : isEdit ? 'Update Node' : 'Create Node'}
          </Button>
        </CardFooter>
      </form>
    </Card>
  );
}
