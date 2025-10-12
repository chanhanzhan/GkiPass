"use client";

import React, { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { Loader2, ArrowLeft, Calendar, ChevronRight, BarChart4, RefreshCw } from 'lucide-react';
import { PlansService } from '@/lib/api/plans';
import { Subscription, Plan } from '@/lib/api/types';

export default function SubscriptionPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [subscription, setSubscription] = useState<Subscription | null>(null);
  const [plan, setPlan] = useState<Plan | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const fetchData = async () => {
      try {
        setLoading(true);
        const plansService = new PlansService();
        
        // Get current subscription
        const subResponse = await plansService.getCurrentSubscription();
        if (subResponse.success && subResponse.data) {
          setSubscription(subResponse.data);
          
          // Get plan details
          const planResponse = await plansService.getPlan(subResponse.data.planId);
          if (planResponse.success && planResponse.data) {
            setPlan(planResponse.data);
          }
        } else {
          // No active subscription
          router.push('/dashboard/plans');
        }
      } catch (err) {
        console.error("Error fetching data:", err);
        setError("Failed to load subscription data");
      } finally {
        setLoading(false);
      }
    };
    
    fetchData();
  }, [router]);

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
  
  if (loading) {
    return (
      <div className="container mx-auto px-4 py-6 flex justify-center items-center min-h-[300px]">
        <Loader2 className="h-8 w-8 animate-spin text-blue-600 mr-2" />
        <span className="text-lg">Loading subscription data...</span>
      </div>
    );
  }
  
  if (error) {
    return (
      <div className="container mx-auto px-4 py-6">
        <button 
          onClick={() => router.push('/dashboard/plans')}
          className="flex items-center text-blue-600 dark:text-blue-400 hover:underline mb-4"
        >
          <ArrowLeft size={16} className="mr-1" />
          Back to Plans
        </button>
        
        <div className="bg-red-50 dark:bg-red-900/20 border-l-4 border-red-500 p-4">
          <div className="flex">
            <div className="flex-shrink-0">
              <svg className="h-5 w-5 text-red-500" viewBox="0 0 20 20" fill="currentColor">
                <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
              </svg>
            </div>
            <div className="ml-3">
              <p className="text-sm font-medium text-red-800 dark:text-red-200">
                {error}
              </p>
            </div>
          </div>
        </div>
      </div>
    );
  }
  
  if (!subscription || !plan) {
    return (
      <div className="container mx-auto px-4 py-6">
        <h1 className="text-2xl font-bold mb-6 dark:text-white">No Active Subscription</h1>
        <p className="mb-4 dark:text-gray-300">You don't have an active subscription.</p>
        <button
          onClick={() => router.push('/dashboard/plans')}
          className="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700"
        >
          Browse Plans
        </button>
      </div>
    );
  }
  
  const trafficPercentage = calculateTrafficPercentage();
  const isExpired = new Date(subscription.expiresAt) < new Date();
  const daysLeft = isExpired ? 0 : Math.ceil((new Date(subscription.expiresAt).getTime() - Date.now()) / (1000 * 3600 * 24));

  return (
    <div className="container mx-auto px-4 py-6">
      <button 
        onClick={() => router.push('/dashboard/plans')}
        className="flex items-center text-blue-600 dark:text-blue-400 hover:underline mb-4"
      >
        <ArrowLeft size={16} className="mr-1" />
        Back to Plans
      </button>
      
      <div className="flex justify-between items-center mb-6">
        <h1 className="text-2xl font-bold dark:text-white">Your Subscription</h1>
        
        {isExpired ? (
          <button
            onClick={() => router.push('/dashboard/plans')}
            className="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 flex items-center"
          >
            <RefreshCw size={16} className="mr-2" />
            Renew Subscription
          </button>
        ) : (
          <div className={`inline-flex items-center px-3 py-1 rounded-full text-sm font-medium ${
            daysLeft <= 7 ? 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-300' : 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-300'
          }`}>
            {daysLeft <= 7 ? 'Expiring Soon' : 'Active'}
          </div>
        )}
      </div>
      
      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        {/* Plan Details */}
        <div className="md:col-span-2 bg-white dark:bg-zinc-800 rounded-lg shadow p-6">
          <div className="flex justify-between items-center mb-4">
            <h2 className="text-xl font-medium dark:text-white">{plan.name} Plan</h2>
            <span className="text-gray-600 dark:text-gray-400 text-sm capitalize">{plan.billingCycle}</span>
          </div>
          
          {plan.description && (
            <p className="text-gray-600 dark:text-gray-400 mb-6">{plan.description}</p>
          )}
          
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div className="border-l-4 border-blue-500 pl-4">
              <div className="text-sm text-gray-500 dark:text-gray-400">Plan Price</div>
              <div className="text-xl font-bold dark:text-white">${plan.price.toFixed(2)}</div>
            </div>
            
            <div className="border-l-4 border-purple-500 pl-4">
              <div className="text-sm text-gray-500 dark:text-gray-400">Billing Cycle</div>
              <div className="text-xl font-bold dark:text-white capitalize">{plan.billingCycle}</div>
            </div>
            
            <div className="border-l-4 border-green-500 pl-4">
              <div className="text-sm text-gray-500 dark:text-gray-400">Start Date</div>
              <div className="text-xl font-bold dark:text-white">
                {new Date(subscription.startAt).toLocaleDateString()}
              </div>
            </div>
            
            <div className="border-l-4 border-red-500 pl-4">
              <div className="text-sm text-gray-500 dark:text-gray-400">Expiry Date</div>
              <div className="text-xl font-bold dark:text-white">
                {new Date(subscription.expiresAt).toLocaleDateString()}
              </div>
            </div>
          </div>
        </div>
        
        {/* Days Remaining */}
        <div className="bg-white dark:bg-zinc-800 rounded-lg shadow p-6 flex flex-col justify-between">
          <div className="text-center">
            <Calendar className="h-8 w-8 text-blue-600 mx-auto mb-2" />
            <h3 className="text-lg font-medium dark:text-white mb-1">Subscription Status</h3>
            {isExpired ? (
              <div className="text-red-600 dark:text-red-400 font-bold text-xl">Expired</div>
            ) : (
              <>
                <div className="text-3xl font-bold text-blue-600">{daysLeft}</div>
                <div className="text-gray-600 dark:text-gray-400">Days Remaining</div>
              </>
            )}
          </div>
          
          <button
            onClick={() => router.push('/dashboard/plans')}
            className={`mt-4 w-full py-2 px-4 rounded-md flex justify-center items-center ${
              isExpired 
                ? 'bg-blue-600 text-white hover:bg-blue-700' 
                : 'bg-gray-200 text-gray-700 hover:bg-gray-300 dark:bg-zinc-700 dark:text-gray-300 dark:hover:bg-zinc-600'
            }`}
          >
            {isExpired ? 'Renew Now' : 'View Plans'}
            <ChevronRight size={16} className="ml-1" />
          </button>
        </div>
      </div>
      
      <div className="mt-8 grid grid-cols-1 md:grid-cols-2 gap-6">
        {/* Traffic Usage */}
        <div className="bg-white dark:bg-zinc-800 rounded-lg shadow p-6">
          <div className="flex items-center mb-4">
            <BarChart4 className="h-5 w-5 text-blue-600 mr-2" />
            <h3 className="text-lg font-medium dark:text-white">Traffic Usage</h3>
          </div>
          
          <div className="mb-2 flex justify-between">
            <div className="text-gray-600 dark:text-gray-400">
              {formatBytes(subscription.usedTraffic)} used
            </div>
            <div className="text-gray-600 dark:text-gray-400">
              {subscription.traffic === 0 ? 'Unlimited' : formatBytes(subscription.traffic)} total
            </div>
          </div>
          
          {subscription.traffic > 0 && (
            <>
              <div className="w-full bg-gray-200 dark:bg-zinc-700 rounded-full h-3 mb-1">
                <div 
                  className={`h-3 rounded-full ${
                    trafficPercentage > 90 ? 'bg-red-600' : trafficPercentage > 75 ? 'bg-amber-500' : 'bg-green-600'
                  }`}
                  style={{ width: `${trafficPercentage}%` }}
                />
              </div>
              
              <div className="text-xs text-gray-600 dark:text-gray-400">
                {trafficPercentage.toFixed(1)}% used
              </div>
            </>
          )}
        </div>
        
        {/* Resources */}
        <div className="bg-white dark:bg-zinc-800 rounded-lg shadow p-6">
          <h3 className="text-lg font-medium mb-4 dark:text-white">Resources</h3>
          
          <dl className="space-y-3">
            <div className="flex justify-between">
              <dt className="text-gray-600 dark:text-gray-400">Max Bandwidth</dt>
              <dd className="font-medium dark:text-white">
                {formatBandwidth(subscription.maxBandwidth)}
              </dd>
            </div>
            
            <div className="flex justify-between">
              <dt className="text-gray-600 dark:text-gray-400">Max Tunnels</dt>
              <dd className="font-medium dark:text-white">
                {subscription.maxTunnels === 0 ? 'Unlimited' : subscription.maxTunnels}
              </dd>
            </div>
          </dl>
        </div>
      </div>
    </div>
  );
}
