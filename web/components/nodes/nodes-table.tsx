'use client';

import { useState } from 'react';
import Link from 'next/link';
import { 
  CheckCircle, 
  XCircle, 
  AlertTriangle,
  MoreHorizontal, 
  Edit, 
  Trash, 
  Download, 
  RefreshCw 
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { Node } from '@/lib/api/types';

interface NodesTableProps {
  nodes: Node[];
  onDeleteNode: (nodeId: string) => void;
}

export function NodesTable({ nodes, onDeleteNode }: NodesTableProps) {
  const [expandedRow, setExpandedRow] = useState<string | null>(null);
  const [isDeleting, setIsDeleting] = useState<string | null>(null);

  const handleDelete = async (nodeId: string) => {
    setIsDeleting(nodeId);
    try {
      await onDeleteNode(nodeId);
    } finally {
      setIsDeleting(null);
    }
  };

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'online':
        return (
          <div className="flex items-center">
            <CheckCircle className="h-4 w-4 text-green-500 mr-1" />
            <span className="text-green-600">Online</span>
          </div>
        );
      case 'offline':
        return (
          <div className="flex items-center">
            <XCircle className="h-4 w-4 text-gray-500 mr-1" />
            <span className="text-gray-600">Offline</span>
          </div>
        );
      case 'error':
        return (
          <div className="flex items-center">
            <AlertTriangle className="h-4 w-4 text-red-500 mr-1" />
            <span className="text-red-600">Error</span>
          </div>
        );
      default:
        return <span>{status}</span>;
    }
  };

  return (
    <Card>
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b">
              <th className="py-3 px-4 text-left font-medium">Name</th>
              <th className="py-3 px-4 text-left font-medium">Type</th>
              <th className="py-3 px-4 text-left font-medium">Status</th>
              <th className="py-3 px-4 text-left font-medium">IP:Port</th>
              <th className="py-3 px-4 text-left font-medium">Group</th>
              <th className="py-3 px-4 text-right font-medium">Actions</th>
            </tr>
          </thead>
          <tbody>
            {nodes.length === 0 ? (
              <tr>
                <td colSpan={6} className="py-6 text-center text-muted-foreground">
                  No nodes found.
                </td>
              </tr>
            ) : (
              nodes.map((node) => (
                <>
                  <tr 
                    key={node.id} 
                    className="border-b hover:bg-muted/50 cursor-pointer" 
                    onClick={() => setExpandedRow(expandedRow === node.id ? null : node.id)}
                  >
                    <td className="py-3 px-4">{node.name}</td>
                    <td className="py-3 px-4 capitalize">{node.type}</td>
                    <td className="py-3 px-4">{getStatusBadge(node.status)}</td>
                    <td className="py-3 px-4">
                      {node.ip}:{node.port}
                    </td>
                    <td className="py-3 px-4">
                      {node.groupName || "-"}
                    </td>
                    <td className="py-3 px-4 text-right">
                      <div className="flex justify-end gap-2">
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={(e) => {
                            e.stopPropagation();
                            setExpandedRow(expandedRow === node.id ? null : node.id);
                          }}
                        >
                          <MoreHorizontal className="h-4 w-4" />
                        </Button>
                      </div>
                    </td>
                  </tr>
                  {expandedRow === node.id && (
                    <tr key={`${node.id}-expanded`}>
                      <td colSpan={6} className="py-4 px-4 bg-muted/30">
                        <div className="flex flex-col space-y-4">
                          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                            <div>
                              <h4 className="font-medium mb-2">Node Details</h4>
                              <dl className="space-y-2">
                                <div className="flex">
                                  <dt className="w-32 font-medium">ID:</dt>
                                  <dd>{node.id}</dd>
                                </div>
                                <div className="flex">
                                  <dt className="w-32 font-medium">Version:</dt>
                                  <dd>{node.version || "Unknown"}</dd>
                                </div>
                                <div className="flex">
                                  <dt className="w-32 font-medium">Last Seen:</dt>
                                  <dd>
                                    {node.lastSeen
                                      ? new Date(node.lastSeen).toLocaleString()
                                      : "Never"}
                                  </dd>
                                </div>
                                <div className="flex">
                                  <dt className="w-32 font-medium">Created:</dt>
                                  <dd>{new Date(node.createdAt).toLocaleString()}</dd>
                                </div>
                              </dl>
                            </div>
                            <div>
                              <h4 className="font-medium mb-2">Description</h4>
                              <p className="text-muted-foreground">
                                {node.description || "No description provided."}
                              </p>
                              
                              <div className="mt-4 flex flex-wrap gap-2">
                                <Button
                                  size="sm"
                                  variant="outline"
                                  className="gap-1"
                                  asChild
                                >
                                  <Link href={`/nodes/${node.id}/edit`}>
                                    <Edit className="h-4 w-4" />
                                    Edit
                                  </Link>
                                </Button>
                                <Button
                                  size="sm"
                                  variant="outline"
                                  className="gap-1"
                                >
                                  <Download className="h-4 w-4" />
                                  Certificate
                                </Button>
                                <Button
                                  size="sm"
                                  variant="outline"
                                  className="gap-1"
                                >
                                  <RefreshCw className="h-4 w-4" />
                                  Refresh
                                </Button>
                                <Button
                                  size="sm"
                                  variant="destructive"
                                  className="gap-1"
                                  disabled={isDeleting === node.id}
                                  onClick={(e) => {
                                    e.stopPropagation();
                                    handleDelete(node.id);
                                  }}
                                >
                                  <Trash className="h-4 w-4" />
                                  {isDeleting === node.id ? 'Deleting...' : 'Delete'}
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
