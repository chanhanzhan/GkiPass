'use client';

import { useState } from 'react';
import Link from 'next/link';
import { 
  Tunnel as TunnelIcon, 
  ToggleLeft,
  ToggleRight, 
  MoreHorizontal, 
  Edit, 
  Trash, 
  BarChart2, 
  ExternalLink 
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { Tunnel } from '@/lib/api/types';

interface TunnelsTableProps {
  tunnels: Tunnel[];
  onDeleteTunnel: (tunnelId: string) => void;
  onToggleTunnel: (tunnelId: string, enabled: boolean) => void;
}

export function TunnelsTable({ tunnels, onDeleteTunnel, onToggleTunnel }: TunnelsTableProps) {
  const [expandedRow, setExpandedRow] = useState<string | null>(null);
  const [isDeleting, setIsDeleting] = useState<string | null>(null);
  const [isToggling, setIsToggling] = useState<string | null>(null);

  const handleDelete = async (tunnelId: string) => {
    setIsDeleting(tunnelId);
    try {
      await onDeleteTunnel(tunnelId);
    } finally {
      setIsDeleting(null);
    }
  };

  const handleToggle = async (tunnelId: string, currentState: boolean) => {
    setIsToggling(tunnelId);
    try {
      await onToggleTunnel(tunnelId, !currentState);
    } finally {
      setIsToggling(null);
    }
  };

  const getProtocolBadge = (protocol: string) => {
    const colors: Record<string, { bg: string; text: string }> = {
      tcp: { bg: 'bg-blue-100', text: 'text-blue-800' },
      udp: { bg: 'bg-green-100', text: 'text-green-800' },
      http: { bg: 'bg-purple-100', text: 'text-purple-800' },
      https: { bg: 'bg-indigo-100', text: 'text-indigo-800' },
    };
    
    const style = colors[protocol] || { bg: 'bg-gray-100', text: 'text-gray-800' };
    
    return (
      <span className={`${style.bg} ${style.text} px-2 py-1 rounded-full text-xs font-medium uppercase`}>
        {protocol}
      </span>
    );
  };

  const formatBytes = (bytes: number) => {
    if (bytes === 0) return '0 B';
    
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  };

  const getTotalTraffic = (tunnel: Tunnel) => {
    return formatBytes(tunnel.trafficIn + tunnel.trafficOut);
  };

  return (
    <Card>
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b">
              <th className="py-3 px-4 text-left font-medium">Name</th>
              <th className="py-3 px-4 text-left font-medium">Protocol</th>
              <th className="py-3 px-4 text-left font-medium">Port</th>
              <th className="py-3 px-4 text-left font-medium">Traffic</th>
              <th className="py-3 px-4 text-left font-medium">Status</th>
              <th className="py-3 px-4 text-right font-medium">Actions</th>
            </tr>
          </thead>
          <tbody>
            {tunnels.length === 0 ? (
              <tr>
                <td colSpan={6} className="py-6 text-center text-muted-foreground">
                  No tunnels found.
                </td>
              </tr>
            ) : (
              tunnels.map((tunnel) => (
                <>
                  <tr 
                    key={tunnel.id} 
                    className={`border-b hover:bg-muted/50 cursor-pointer ${!tunnel.enabled ? 'opacity-60' : ''}`}
                    onClick={() => setExpandedRow(expandedRow === tunnel.id ? null : tunnel.id)}
                  >
                    <td className="py-3 px-4">{tunnel.name}</td>
                    <td className="py-3 px-4">{getProtocolBadge(tunnel.protocol)}</td>
                    <td className="py-3 px-4">{tunnel.localPort}</td>
                    <td className="py-3 px-4">{getTotalTraffic(tunnel)}</td>
                    <td className="py-3 px-4">
                      {tunnel.enabled ? (
                        <span className="flex items-center text-green-600">
                          <ToggleRight className="h-4 w-4 mr-1" />
                          Active
                        </span>
                      ) : (
                        <span className="flex items-center text-gray-500">
                          <ToggleLeft className="h-4 w-4 mr-1" />
                          Disabled
                        </span>
                      )}
                    </td>
                    <td className="py-3 px-4 text-right">
                      <div className="flex justify-end gap-2">
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={(e) => {
                            e.stopPropagation();
                            setExpandedRow(expandedRow === tunnel.id ? null : tunnel.id);
                          }}
                        >
                          <MoreHorizontal className="h-4 w-4" />
                        </Button>
                      </div>
                    </td>
                  </tr>
                  {expandedRow === tunnel.id && (
                    <tr key={`${tunnel.id}-expanded`}>
                      <td colSpan={6} className="py-4 px-4 bg-muted/30">
                        <div className="flex flex-col space-y-4">
                          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                            <div>
                              <h4 className="font-medium mb-2">Tunnel Details</h4>
                              <dl className="space-y-2">
                                <div className="flex">
                                  <dt className="w-32 font-medium">ID:</dt>
                                  <dd>{tunnel.id}</dd>
                                </div>
                                <div className="flex">
                                  <dt className="w-32 font-medium">Entry Group:</dt>
                                  <dd>{tunnel.entryGroupId}</dd>
                                </div>
                                <div className="flex">
                                  <dt className="w-32 font-medium">Exit Group:</dt>
                                  <dd>{tunnel.exitGroupId}</dd>
                                </div>
                                <div className="flex">
                                  <dt className="w-32 font-medium">Connections:</dt>
                                  <dd>{tunnel.connectionCount}</dd>
                                </div>
                                <div className="flex">
                                  <dt className="w-32 font-medium">Traffic:</dt>
                                  <dd>
                                    In: {formatBytes(tunnel.trafficIn)}, Out: {formatBytes(tunnel.trafficOut)}
                                  </dd>
                                </div>
                              </dl>
                            </div>
                            <div>
                              <h4 className="font-medium mb-2">Targets</h4>
                              <div className="space-y-2">
                                {Array.isArray(tunnel.targets) ? (
                                  tunnel.targets.map((target, index) => (
                                    <div 
                                      key={index} 
                                      className="flex items-center justify-between bg-muted/50 p-2 rounded-md"
                                    >
                                      <span>
                                        {target.host}:{target.port}
                                      </span>
                                      {target.weight > 0 && (
                                        <span className="text-xs bg-muted px-2 py-0.5 rounded">
                                          Weight: {target.weight}
                                        </span>
                                      )}
                                    </div>
                                  ))
                                ) : (
                                  <p className="text-muted-foreground">No targets defined</p>
                                )}
                              </div>
                              
                              <div className="mt-4 flex flex-wrap gap-2">
                                <Button
                                  size="sm"
                                  variant="outline"
                                  className="gap-1"
                                  asChild
                                >
                                  <Link href={`/tunnels/${tunnel.id}/edit`}>
                                    <Edit className="h-4 w-4" />
                                    Edit
                                  </Link>
                                </Button>
                                <Button
                                  size="sm"
                                  variant="outline"
                                  className="gap-1"
                                  onClick={(e) => {
                                    e.stopPropagation();
                                    handleToggle(tunnel.id, tunnel.enabled);
                                  }}
                                  disabled={isToggling === tunnel.id}
                                >
                                  {tunnel.enabled ? (
                                    <>
                                      <ToggleLeft className="h-4 w-4" />
                                      {isToggling === tunnel.id ? 'Disabling...' : 'Disable'}
                                    </>
                                  ) : (
                                    <>
                                      <ToggleRight className="h-4 w-4" />
                                      {isToggling === tunnel.id ? 'Enabling...' : 'Enable'}
                                    </>
                                  )}
                                </Button>
                                <Button
                                  size="sm"
                                  variant="outline"
                                  className="gap-1"
                                  asChild
                                >
                                  <Link href={`/tunnels/${tunnel.id}/stats`}>
                                    <BarChart2 className="h-4 w-4" />
                                    Stats
                                  </Link>
                                </Button>
                                <Button
                                  size="sm"
                                  variant="destructive"
                                  className="gap-1"
                                  disabled={isDeleting === tunnel.id}
                                  onClick={(e) => {
                                    e.stopPropagation();
                                    handleDelete(tunnel.id);
                                  }}
                                >
                                  <Trash className="h-4 w-4" />
                                  {isDeleting === tunnel.id ? 'Deleting...' : 'Delete'}
                                </Button>
                              </div>
                            </div>
                          </div>
                        </div>
                      </td>
                    </tr>
                  )}
                </>
              ))
            )}
          </tbody>
        </table>
      </div>
    </Card>
  );
}
