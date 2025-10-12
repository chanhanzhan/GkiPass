"use client";

import React from 'react';
import { useRouter } from 'next/navigation';
import { Package, Check, AlertTriangle } from 'lucide-react';
import { Subscription, Plan } from '@/lib/api/types';

interface SubscriptionCardProps {
  subscription?: Subscription;
  plan?: Plan;
}

const SubscriptionCard: React.FC<SubscriptionCardProps> = ({ subscription, plan }) => {
  const router = useRouter();
  
  if (!subscription && !plan) {
    return (
      <div className="bg-white dark:bg-zinc-800 shadow rounded-lg p-6">
        <div className="flex items-center justify-center space-x-2">
          <AlertTriangle className="h-6 w-6 text-amber-500" />
          <span className="text-gray-700 dark:text-gray-300">No active subscription</span>
        </div>
        <div className="mt-4 text-center">
          <button
            onClick={() => router.push('/dashboard/plans')}
            className="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700"
          >
            Browse Plans
          </button>
        </div>
      </div>
    );
  }
  
  const formatBytes = (bytes: number) => {
    if (bytes === 0) return '0 GB';
    
    const gb = bytes / (1024 * 1024 * 1024);
    return `${gb.toFixed(0)} GB`;
  };
  
  const formatBandwidth = (bps: number) => {
    if (bps === 0) return 'Unlimited';
    
    if (bps >= 1000000000) {
      return `${(bps / 1000000000).toFixed(0)} Gbps`;
    } else if (bps >= 1000000) {
      return `${(bps / 1000000).toFixed(0)} Mbps`;
    } else {
      return `${(bps / 1000).toFixed(0)} Kbps`;
    }
  };
  
  const calculateTrafficPercentage = () => {
    if (!subscription) return 0;
    if (subscription.traffic === 0) return 0; // Unlimited
    
    return Math.min(100, (subscription.usedTraffic / subscription.traffic) * 100);
  };
  
  const trafficPercentage = calculateTrafficPercentage();
  const isExpired = subscription && new Date(subscription.expiresAt) < new Date();
  const daysLeft = subscription 
    ? Math.max(0, Math.ceil((new Date(subscription.expiresAt).getTime() - Date.now()) / (1000 * 3600 * 24)))
    : 0;
  
  return (
    <div className="bg-white dark:bg-zinc-800 shadow rounded-lg overflow-hidden">
      <div className="bg-blue-600 p-4">
        <div className="flex justify-between items-center">
          <h3 className="text-lg font-medium text-white">
            {plan?.name || 'Your Subscription'}
          </h3>
          <Package className="h-6 w-6 text-white" />
        </div>
        {isExpired && (
          <div className="mt-2 inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-red-100 text-red-800">
            Expired
          </div>
        )}
      </div>
      
      <div className="p-6">
        <div className="space-y-4">
          {/* Traffic Usage */}
          <div>
            <div className="flex justify-between text-sm mb-1">
              <span className="text-gray-700 dark:text-gray-300">Traffic</span>
              <span className="text-gray-700 dark:text-gray-300">
                {subscription ? (
                  <>
                    {formatBytes(subscription.usedTraffic)} / {subscription.traffic === 0 ? 'Unlimited' : formatBytes(subscription.traffic)}
                  </>
                ) : (
                  plan && (plan.maxTraffic === 0 ? 'Unlimited' : formatBytes(plan.maxTraffic))
                )}
              </span>
            </div>
            {subscription && subscription.traffic > 0 && (
              <div className="w-full bg-gray-200 dark:bg-zinc-700 rounded-full h-2.5">
                <div 
                  className={`h-2.5 rounded-full ${
                    trafficPercentage > 90 ? 'bg-red-600' : trafficPercentage > 75 ? 'bg-amber-500' : 'bg-green-600'
                  }`}
                  style={{ width: `${trafficPercentage}%` }}
                />
              </div>
            )}
          </div>
          
          {/* Bandwidth Limit */}
          <div className="flex justify-between">
            <span className="text-gray-700 dark:text-gray-300">Bandwidth Limit</span>
            <span className="text-gray-700 dark:text-gray-300">
              {subscription 
                ? formatBandwidth(subscription.maxBandwidth) 
                : (plan && formatBandwidth(plan.maxBandwidth))}
            </span>
          </div>
          
          {/* Max Tunnels */}
          <div className="flex justify-between">
            <span className="text-gray-700 dark:text-gray-300">Max Tunnels</span>
            <span className="text-gray-700 dark:text-gray-300">
              {subscription 
                ? (subscription.maxTunnels === 0 ? 'Unlimited' : subscription.maxTunnels)
                : (plan && (plan.maxConnections === 0 ? 'Unlimited' : plan.maxConnections))}
            </span>
          </div>
          
          {/* Validity Period */}
          {subscription && (
            <div className="flex justify-between">
              <span className="text-gray-700 dark:text-gray-300">Validity</span>
              <span className="text-gray-700 dark:text-gray-300">
                {isExpired 
                  ? 'Expired' 
                  : `${daysLeft} days left`}
              </span>
            </div>
          )}
          
          {plan && !subscription && (
            <>
              <div className="flex justify-between">
                <span className="text-gray-700 dark:text-gray-300">Price</span>
                <span className="text-gray-700 dark:text-gray-300">
                  ${plan.price.toFixed(2)} / {plan.billingCycle}
                </span>
              </div>
            
              <div className="mt-4">
                <ul className="space-y-2">
                  <li className="flex items-start">
                    <Check className="h-5 w-5 text-green-500 mr-2 flex-shrink-0" />
                    <span className="text-sm text-gray-700 dark:text-gray-300">
                      {plan.maxTraffic === 0 ? 'Unlimited traffic' : `${formatBytes(plan.maxTraffic)} traffic`}
                    </span>
                  </li>
                  <li className="flex items-start">
                    <Check className="h-5 w-5 text-green-500 mr-2 flex-shrink-0" />
                    <span className="text-sm text-gray-700 dark:text-gray-300">
                      {plan.maxBandwidth === 0 ? 'Unlimited bandwidth' : `${formatBandwidth(plan.maxBandwidth)} bandwidth`}
                    </span>
                  </li>
                  <li className="flex items-start">
                    <Check className="h-5 w-5 text-green-500 mr-2 flex-shrink-0" />
                    <span className="text-sm text-gray-700 dark:text-gray-300">
                      {plan.maxConnections === 0 ? 'Unlimited connections' : `${plan.maxConnections} max connections`}
                    </span>
                  </li>
                </ul>
              </div>
            </>
          )}
        </div>
        
        <div className="mt-6">
          {subscription ? (
            isExpired ? (
              <button
                onClick={() => router.push('/dashboard/plans')}
                className="w-full px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700"
              >
                Renew Subscription
              </button>
            ) : (
              <button
                onClick={() => router.push('/dashboard/subscription')}
                className="w-full px-4 py-2 bg-gray-200 text-gray-800 dark:bg-zinc-700 dark:text-white rounded-md hover:bg-gray-300 dark:hover:bg-zinc-600"
              >
                View Details
              </button>
            )
          ) : plan && (
            <button
              onClick={() => router.push(`/dashboard/plans/subscribe/${plan.id}`)}
              className="w-full px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700"
            >
              Subscribe Now
            </button>
          )}
        </div>
      </div>
    </div>
  );
};

export default SubscriptionCard;
