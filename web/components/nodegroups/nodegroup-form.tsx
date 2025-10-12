'use client';

import { useState, useEffect } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from '@/components/ui/card';
import { NodeGroup } from '@/lib/api/types';

// Form validation schema
const nodeGroupSchema = z.object({
  name: z.string().min(1, 'Name is required').max(100),
  type: z.enum(['entry', 'exit']),
  description: z.string().optional(),
});

type NodeGroupFormValues = z.infer<typeof nodeGroupSchema>;

interface NodeGroupFormProps {
  group?: NodeGroup;
  onSubmit: (data: NodeGroupFormValues) => Promise<void>;
  isLoading?: boolean;
}

export function NodeGroupForm({ group, onSubmit, isLoading }: NodeGroupFormProps) {
  const [error, setError] = useState<string | null>(null);

  // Initialize form
  const {
    register,
    handleSubmit,
    formState: { errors },
    reset,
  } = useForm<NodeGroupFormValues>({
    resolver: zodResolver(nodeGroupSchema),
    defaultValues: {
      name: '',
      type: 'entry',
      description: '',
    },
  });

  // Set form defaults when group prop changes
  useEffect(() => {
    if (group) {
      reset({
        name: group.name,
        type: group.type as 'entry' | 'exit',
        description: group.description || '',
      });
    }
  }, [group, reset]);

  const handleFormSubmit = async (data: NodeGroupFormValues) => {
    setError(null);
    try {
      await onSubmit(data);
    } catch (err: any) {
      setError(err.message || 'An error occurred while saving the node group');
    }
  };

  const isEdit = !!group;

  return (
    <Card className="w-full">
      <CardHeader>
        <CardTitle>{isEdit ? 'Edit Node Group' : 'Create Node Group'}</CardTitle>
      </CardHeader>
      <form onSubmit={handleSubmit(handleFormSubmit)}>
        <CardContent className="space-y-4">
          {error && (
            <div className="p-3 text-sm bg-red-50 text-red-600 rounded-md">{error}</div>
          )}
          
          <div className="space-y-2">
            <label htmlFor="name" className="text-sm font-medium">
              Group Name
            </label>
            <Input
              id="name"
              placeholder="Enter group name"
              {...register('name')}
              disabled={isLoading}
            />
            {errors.name && (
              <p className="text-sm text-red-500">{errors.name.message}</p>
            )}
          </div>
          
          <div className="space-y-2">
            <label htmlFor="type" className="text-sm font-medium">
              Group Type
            </label>
            <select
              id="type"
              className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
              {...register('type')}
              disabled={isLoading || isEdit}
            >
              <option value="entry">Entry (Ingress)</option>
              <option value="exit">Exit (Egress)</option>
            </select>
            {errors.type && (
              <p className="text-sm text-red-500">{errors.type.message}</p>
            )}
            {isEdit && (
              <p className="text-xs text-muted-foreground">
                Group type cannot be changed after creation.
              </p>
            )}
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
            {isLoading ? 'Saving...' : isEdit ? 'Update Group' : 'Create Group'}
          </Button>
        </CardFooter>
      </form>
    </Card>
  );
}
