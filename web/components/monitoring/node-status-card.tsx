"use client";

import React from 'react';
import { cn } from '@/lib/utils';

interface NodeStatusCardProps {
  id: string;
  name: string;
  status: 'online' | 'offline' | 'error' | 'busy';
  cpuUsage?: number;
  memoryUsage?: number;
  uptime?: string;
  lastSeen?: string;
  connections?: number;
  onClick?: () => void;
}

const statusColors = {
  online: "bg-green-500",
  offline: "bg-gray-500",
  error: "bg-red-500",
  busy: "bg-yellow-500"
};

const statusLabels = {
  online: "Online",
  offline: "Offline",
  error: "Error",
  busy: "Busy"
};

const NodeStatusCard: React.FC<NodeStatusCardProps> = ({ 
  id,
  name,
  status,
  cpuUsage,
  memoryUsage,
  uptime,
  lastSeen,
  connections,
  onClick
}) => {
  return (
    <div 
      className={cn(
        "p-4 bg-white dark:bg-zinc-800 rounded-lg shadow",
        "border-l-4",
        `border-l-${status === 'online' ? 'green-500' : status === 'offline' ? 'gray-500' : status === 'error' ? 'red-500' : 'yellow-500'}`,
        onClick && "cursor-pointer hover:shadow-md transition-shadow"
      )}
      onClick={onClick}
    >
      <div className="flex justify-between items-start">
        <div>
          <h3 className="font-medium text-lg dark:text-white">{name}</h3>
          <p className="text-sm text-gray-500 dark:text-gray-400">ID: {id}</p>
        </div>
        <div className="flex items-center">
          <div className={cn(
            "h-3 w-3 rounded-full mr-2",
            statusColors[status]
          )} />
          <span className="text-sm font-medium dark:text-gray-300">{statusLabels[status]}</span>
        </div>
      </div>
      
      <div className="mt-4 grid grid-cols-2 gap-2">
        {cpuUsage !== undefined && (
          <div className="flex flex-col">
            <span className="text-xs text-gray-500 dark:text-gray-400">CPU</span>
            <span className="font-medium dark:text-gray-200">{cpuUsage}%</span>
            <div className="w-full bg-gray-200 dark:bg-zinc-700 rounded-full h-1.5 mt-1">
              <div 
                className={cn(
                  "h-1.5 rounded-full",
                  cpuUsage > 80 ? "bg-red-500" : cpuUsage > 50 ? "bg-yellow-500" : "bg-green-500"
                )}
                style={{ width: `${cpuUsage}%` }}
              />
            </div>
          </div>
        )}
        
        {memoryUsage !== undefined && (
          <div className="flex flex-col">
            <span className="text-xs text-gray-500 dark:text-gray-400">Memory</span>
            <span className="font-medium dark:text-gray-200">{memoryUsage}%</span>
            <div className="w-full bg-gray-200 dark:bg-zinc-700 rounded-full h-1.5 mt-1">
              <div 
                className={cn(
                  "h-1.5 rounded-full",
                  memoryUsage > 80 ? "bg-red-500" : memoryUsage > 50 ? "bg-yellow-500" : "bg-green-500"
                )}
                style={{ width: `${memoryUsage}%` }}
              />
            </div>
          </div>
        )}
      </div>
      
      <div className="mt-3 text-xs text-gray-500 dark:text-gray-400 grid grid-cols-2 gap-2">
        {uptime && <div>Uptime: <span className="font-medium dark:text-gray-300">{uptime}</span></div>}
        {lastSeen && <div>Last seen: <span className="font-medium dark:text-gray-300">{lastSeen}</span></div>}
        {connections !== undefined && <div>Connections: <span className="font-medium dark:text-gray-300">{connections}</span></div>}
      </div>
    </div>
  );
};

export default NodeStatusCard;
