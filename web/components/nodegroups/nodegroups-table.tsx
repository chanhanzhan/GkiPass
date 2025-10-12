'use client';

import { useState } from 'react';
import Link from 'next/link';
import { 
  ArrowRight, 
  ArrowLeft, 
  MoreHorizontal, 
  Edit, 
  Trash, 
  Server, 
  Settings 
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { NodeGroup } from '@/lib/api/types';

interface NodeGroupsTableProps {
  groups: NodeGroup[];
  onDeleteGroup: (groupId: string) => void;
}

export function NodeGroupsTable({ groups, onDeleteGroup }: NodeGroupsTableProps) {
  const [expandedRow, setExpandedRow] = useState<string | null>(null);
  const [isDeleting, setIsDeleting] = useState<string | null>(null);

  const handleDelete = async (groupId: string) => {
    setIsDeleting(groupId);
    try {
      await onDeleteGroup(groupId);
    } finally {
      setIsDeleting(null);
    }
  };

  const getGroupTypeBadge = (type: string) => {
    switch (type) {
      case 'entry':
        return (
          <div className="flex items-center">
            <ArrowRight className="h-4 w-4 text-blue-500 mr-1" />
            <span className="text-blue-600">Entry</span>
          </div>
        );
      case 'exit':
        return (
          <div className="flex items-center">
            <ArrowLeft className="h-4 w-4 text-green-500 mr-1" />
            <span className="text-green-600">Exit</span>
          </div>
        );
      default:
        return <span>{type}</span>;
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
              <th className="py-3 px-4 text-left font-medium">Nodes</th>
              <th className="py-3 px-4 text-left font-medium">Created</th>
              <th className="py-3 px-4 text-right font-medium">Actions</th>
            </tr>
          </thead>
          <tbody>
            {groups.length === 0 ? (
              <tr>
                <td colSpan={5} className="py-6 text-center text-muted-foreground">
                  No node groups found.
                </td>
              </tr>
            ) : (
              groups.map((group) => (
                <>
                  <tr 
                    key={group.id} 
                    className="border-b hover:bg-muted/50 cursor-pointer" 
                    onClick={() => setExpandedRow(expandedRow === group.id ? null : group.id)}
                  >
                    <td className="py-3 px-4">{group.name}</td>
                    <td className="py-3 px-4">{getGroupTypeBadge(group.type)}</td>
                    <td className="py-3 px-4">{group.nodeCount}</td>
                    <td className="py-3 px-4">
                      {new Date(group.createdAt).toLocaleDateString()}
                    </td>
                    <td className="py-3 px-4 text-right">
                      <div className="flex justify-end gap-2">
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={(e) => {
                            e.stopPropagation();
                            setExpandedRow(expandedRow === group.id ? null : group.id);
                          }}
                        >
                          <MoreHorizontal className="h-4 w-4" />
                        </Button>
                      </div>
                    </td>
                  </tr>
                  {expandedRow === group.id && (
                    <tr key={`${group.id}-expanded`}>
                      <td colSpan={5} className="py-4 px-4 bg-muted/30">
                        <div className="flex flex-col space-y-4">
                          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                            <div>
                              <h4 className="font-medium mb-2">Group Details</h4>
                              <dl className="space-y-2">
                                <div className="flex">
                                  <dt className="w-32 font-medium">ID:</dt>
                                  <dd>{group.id}</dd>
                                </div>
                                <div className="flex">
                                  <dt className="w-32 font-medium">Type:</dt>
                                  <dd className="capitalize">{group.type}</dd>
                                </div>
                                <div className="flex">
                                  <dt className="w-32 font-medium">Node Count:</dt>
                                  <dd>{group.nodeCount}</dd>
                                </div>
                                <div className="flex">
                                  <dt className="w-32 font-medium">Created:</dt>
                                  <dd>{new Date(group.createdAt).toLocaleString()}</dd>
                                </div>
                              </dl>
                            </div>
                            <div>
                              <h4 className="font-medium mb-2">Description</h4>
                              <p className="text-muted-foreground">
                                {group.description || "No description provided."}
                              </p>
                              
                              <div className="mt-4 flex flex-wrap gap-2">
                                <Button
                                  size="sm"
                                  variant="outline"
                                  className="gap-1"
                                  asChild
                                >
                                  <Link href={`/nodegroups/${group.id}`}>
                                    <Server className="h-4 w-4" />
                                    View Nodes
                                  </Link>
                                </Button>
                                <Button
                                  size="sm"
                                  variant="outline"
                                  className="gap-1"
                                  asChild
                                >
                                  <Link href={`/nodegroups/${group.id}/edit`}>
                                    <Edit className="h-4 w-4" />
                                    Edit
                                  </Link>
                                </Button>
                                <Button
                                  size="sm"
                                  variant="outline"
                                  className="gap-1"
                                  asChild
                                >
                                  <Link href={`/nodegroups/${group.id}/config`}>
                                    <Settings className="h-4 w-4" />
                                    Configuration
                                  </Link>
                                </Button>
                                <Button
                                  size="sm"
                                  variant="destructive"
                                  className="gap-1"
                                  disabled={isDeleting === group.id || group.nodeCount > 0}
                                  onClick={(e) => {
                                    e.stopPropagation();
                                    handleDelete(group.id);
                                  }}
                                >
                                  <Trash className="h-4 w-4" />
                                  {isDeleting === group.id ? 'Deleting...' : 'Delete'}
                                </Button>
                              </div>
                              {group.nodeCount > 0 && (
                                <p className="text-xs text-amber-600 mt-2">
                                  Group cannot be deleted while it contains nodes
                                </p>
                              )}
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
