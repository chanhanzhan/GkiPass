'use client';

import { useState, useEffect } from 'react';
import { useForm, useFieldArray } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { Plus, Trash } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from '@/components/ui/card';
import { Tunnel, NodeGroup, TunnelTarget } from '@/lib/api/types';

// Form validation schema
const tunnelSchema = z.object({
  name: z.string().min(1, 'Name is required').max(100),
  protocol: z.enum(['tcp', 'udp', 'http', 'https']),
  entryGroupId: z.string().min(1, 'Entry group is required'),
  exitGroupId: z.string().min(1, 'Exit group is required'),
  localPort: z.coerce.number().int().min(1024, 'Port must be 1024 or higher').max(65535, 'Port must be 65535 or lower'),
  targets: z.array(z.object({
    host: z.string().min(1, 'Host is required'),
    port: z.coerce.number().int().min(1, 'Port is required').max(65535, 'Port must be 65535 or lower'),
    weight: z.coerce.number().int().min(0).default(1),
  })).min(1, 'At least one target is required'),
  enabled: z.boolean().default(true),
  description: z.string().optional(),
});

type TunnelFormValues = z.infer<typeof tunnelSchema>;

interface TunnelFormProps {
  tunnel?: Tunnel;
  entryGroups?: NodeGroup[];
  exitGroups?: NodeGroup[];
  onSubmit: (data: TunnelFormValues) => Promise<void>;
  isLoading?: boolean;
}

export function TunnelForm({ tunnel, entryGroups, exitGroups, onSubmit, isLoading }: TunnelFormProps) {
  const [error, setError] = useState<string | null>(null);

  // Initialize form
  const {
    register,
    handleSubmit,
    formState: { errors },
    reset,
    control,
    watch,
  } = useForm<TunnelFormValues>({
    resolver: zodResolver(tunnelSchema),
    defaultValues: {
      name: '',
      protocol: 'tcp',
      entryGroupId: '',
      exitGroupId: '',
      localPort: 8000,
      targets: [{ host: '', port: 80, weight: 1 }],
      enabled: true,
      description: '',
    },
  });

  // Field array for dynamic targets
  const { fields, append, remove } = useFieldArray({
    control,
    name: 'targets',
  });

  // Set form defaults when tunnel prop changes
  useEffect(() => {
    if (tunnel) {
      const targets = Array.isArray(tunnel.targets) 
        ? tunnel.targets
        : typeof tunnel.targets === 'string'
          ? JSON.parse(tunnel.targets)
          : [{ host: '', port: 80, weight: 1 }];
      
      reset({
        name: tunnel.name,
        protocol: tunnel.protocol as 'tcp' | 'udp' | 'http' | 'https',
        entryGroupId: tunnel.entryGroupId,
        exitGroupId: tunnel.exitGroupId,
        localPort: tunnel.localPort,
        targets,
        enabled: tunnel.enabled,
        description: tunnel.description || '',
      });
    }
  }, [tunnel, reset]);

  const handleFormSubmit = async (data: TunnelFormValues) => {
    setError(null);
    try {
      await onSubmit(data);
    } catch (err: any) {
      setError(err.message || 'An error occurred while saving the tunnel');
    }
  };

  const isEdit = !!tunnel;
  const watchProtocol = watch('protocol');
  
  return (
    <Card className="w-full">
      <CardHeader>
        <CardTitle>{isEdit ? 'Edit Tunnel' : 'Create Tunnel'}</CardTitle>
      </CardHeader>
      <form onSubmit={handleSubmit(handleFormSubmit)}>
        <CardContent className="space-y-4">
          {error && (
            <div className="p-3 text-sm bg-red-50 text-red-600 rounded-md">{error}</div>
          )}
          
          <div className="space-y-2">
            <label htmlFor="name" className="text-sm font-medium">
              Tunnel Name
            </label>
            <Input
              id="name"
              placeholder="Enter tunnel name"
              {...register('name')}
              disabled={isLoading}
            />
            {errors.name && (
              <p className="text-sm text-red-500">{errors.name.message}</p>
            )}
          </div>
          
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div className="space-y-2">
              <label htmlFor="protocol" className="text-sm font-medium">
                Protocol
              </label>
              <select
                id="protocol"
                className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                {...register('protocol')}
                disabled={isLoading}
              >
                <option value="tcp">TCP</option>
                <option value="udp">UDP</option>
                <option value="http">HTTP</option>
                <option value="https">HTTPS</option>
              </select>
              {errors.protocol && (
                <p className="text-sm text-red-500">{errors.protocol.message}</p>
              )}
            </div>
            
            <div className="space-y-2">
              <label htmlFor="localPort" className="text-sm font-medium">
                Local Port
              </label>
              <Input
                id="localPort"
                type="number"
                placeholder="Enter port (1024-65535)"
                {...register('localPort')}
                disabled={isLoading}
              />
              {errors.localPort && (
                <p className="text-sm text-red-500">{errors.localPort.message}</p>
              )}
            </div>
          </div>
          
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div className="space-y-2">
              <label htmlFor="entryGroupId" className="text-sm font-medium">
                Entry Group
              </label>
              <select
                id="entryGroupId"
                className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                {...register('entryGroupId')}
                disabled={isLoading}
              >
                <option value="">Select entry group</option>
                {entryGroups?.map((group) => (
                  <option key={group.id} value={group.id}>
                    {group.name}
                  </option>
                ))}
              </select>
              {errors.entryGroupId && (
                <p className="text-sm text-red-500">{errors.entryGroupId.message}</p>
              )}
            </div>
            
            <div className="space-y-2">
              <label htmlFor="exitGroupId" className="text-sm font-medium">
                Exit Group
              </label>
              <select
                id="exitGroupId"
                className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                {...register('exitGroupId')}
                disabled={isLoading}
              >
                <option value="">Select exit group</option>
                {exitGroups?.map((group) => (
                  <option key={group.id} value={group.id}>
                    {group.name}
                  </option>
                ))}
              </select>
              {errors.exitGroupId && (
                <p className="text-sm text-red-500">{errors.exitGroupId.message}</p>
              )}
            </div>
          </div>
          
          <div className="space-y-2">
            <div className="flex justify-between items-center">
              <label className="text-sm font-medium">
                Targets
              </label>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => append({ host: '', port: 80, weight: 1 })}
              >
                <Plus className="h-4 w-4 mr-1" />
                Add Target
              </Button>
            </div>
            
            <div className="space-y-3">
              {fields.map((field, index) => (
                <div key={field.id} className="p-3 border rounded-md">
                  <div className="grid grid-cols-12 gap-2">
                    <div className="col-span-6">
                      <label className="text-xs font-medium">
                        Host
                      </label>
                      <Input
                        {...register(`targets.${index}.host`)}
                        placeholder="hostname or IP"
                        disabled={isLoading}
                      />
                      {errors.targets?.[index]?.host && (
                        <p className="text-xs text-red-500">
                          {errors.targets[index]?.host?.message}
                        </p>
                      )}
                    </div>
                    
                    <div className="col-span-3">
                      <label className="text-xs font-medium">
                        Port
                      </label>
                      <Input
                        type="number"
                        {...register(`targets.${index}.port`)}
                        placeholder="port"
                        disabled={isLoading}
                      />
                      {errors.targets?.[index]?.port && (
                        <p className="text-xs text-red-500">
                          {errors.targets[index]?.port?.message}
                        </p>
                      )}
                    </div>
                    
                    <div className="col-span-2">
                      <label className="text-xs font-medium">
                        Weight
                      </label>
                      <Input
                        type="number"
                        {...register(`targets.${index}.weight`)}
                        placeholder="weight"
                        disabled={isLoading}
                      />
                    </div>
                    
                    <div className="col-span-1 flex items-end justify-center">
                      <Button
                        type="button"
                        size="icon"
                        variant="ghost"
                        className="h-10"
                        onClick={() => fields.length > 1 && remove(index)}
                        disabled={isLoading || fields.length <= 1}
                      >
                        <Trash className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                </div>
              ))}
              
              {errors.targets && !Array.isArray(errors.targets) && (
                <p className="text-sm text-red-500">{errors.targets.message}</p>
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
          
          <div className="flex items-center space-x-2">
            <input
              type="checkbox"
              id="enabled"
              className="h-4 w-4 rounded border-gray-300 text-primary focus:ring-primary"
              {...register('enabled')}
              disabled={isLoading}
            />
            <label htmlFor="enabled" className="text-sm font-medium">
              Enable tunnel immediately
            </label>
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
            {isLoading ? 'Saving...' : isEdit ? 'Update Tunnel' : 'Create Tunnel'}
          </Button>
        </CardFooter>
      </form>
    </Card>
  );
}
