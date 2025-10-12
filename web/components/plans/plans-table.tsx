"use client";

import React, { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { 
  Pencil, 
  Trash, 
  ChevronDown, 
  AlertCircle, 
  Check, 
  X, 
  Package,
  Loader2 
} from 'lucide-react';
import { Plan } from '@/lib/api/types';
import { PlansService } from '@/lib/api/plans';

interface PlansTableProps {
  initialPlans?: Plan[];
  isAdmin?: boolean;
}

const PlansTable: React.FC<PlansTableProps> = ({ initialPlans = [], isAdmin = false }) => {
  const router = useRouter();
  const [plans, setPlans] = useState<Plan[]>(initialPlans);
  const [loading, setLoading] = useState<boolean>(initialPlans.length === 0);
  const [error, setError] = useState<string | null>(null);
  const [sortBy, setSortBy] = useState<string>('name');
  const [sortDirection, setSortDirection] = useState<'asc' | 'desc'>('asc');
  const [actionInProgress, setActionInProgress] = useState<{[key: string]: boolean}>({});

  const fetchPlans = async () => {
    try {
      setLoading(true);
      setError(null);
      const plansService = new PlansService();
      const response = await plansService.getPlans();
      if (response.success && response.data) {
        setPlans(response.data);
      } else {
        setError(response.message || "Failed to fetch plans");
      }
    } catch (err) {
      setError("An error occurred while fetching plans");
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (initialPlans.length === 0) {
      fetchPlans();
    }
  }, [initialPlans.length]);

  const handleSort = (field: string) => {
    if (sortBy === field) {
      setSortDirection(sortDirection === 'asc' ? 'desc' : 'asc');
    } else {
      setSortBy(field);
      setSortDirection('asc');
    }
  };

  const sortedPlans = [...plans].sort((a, b) => {
    let comparison = 0;
    
    if (sortBy === 'name') {
      comparison = a.name.localeCompare(b.name);
    } else if (sortBy === 'price') {
      comparison = a.price - b.price;
    } else if (sortBy === 'billingCycle') {
      comparison = a.billingCycle.localeCompare(b.billingCycle);
    }
    
    return sortDirection === 'asc' ? comparison : -comparison;
  });

  const togglePlanStatus = async (plan: Plan) => {
    if (!isAdmin) return;
    
    try {
      setActionInProgress({ ...actionInProgress, [plan.id]: true });
      const plansService = new PlansService();
      const response = await plansService.togglePlanStatus(plan.id, !plan.enabled);
      
      if (response.success) {
        setPlans(plans.map(p => p.id === plan.id ? { ...p, enabled: !p.enabled } : p));
      } else {
        setError(`Failed to update plan status: ${response.message}`);
      }
    } catch (err) {
      setError("An error occurred while updating plan status");
      console.error(err);
    } finally {
      setActionInProgress({ ...actionInProgress, [plan.id]: false });
    }
  };

  const deletePlan = async (plan: Plan) => {
    if (!isAdmin) return;
    
    if (!window.confirm(`Are you sure you want to delete plan ${plan.name}?`)) {
      return;
    }

    try {
      setActionInProgress({ ...actionInProgress, [plan.id]: true });
      const plansService = new PlansService();
      const response = await plansService.deletePlan(plan.id);
      
      if (response.success) {
        setPlans(plans.filter(p => p.id !== plan.id));
      } else {
        setError(`Failed to delete plan: ${response.message}`);
      }
    } catch (err) {
      setError("An error occurred while deleting plan");
      console.error(err);
    } finally {
      setActionInProgress({ ...actionInProgress, [plan.id]: false });
    }
  };

  const subscribeToPlan = (plan: Plan) => {
    router.push(`/dashboard/plans/subscribe/${plan.id}`);
  };

  if (loading) {
    return (
      <div className="flex justify-center items-center min-h-[200px]">
        <Loader2 className="h-6 w-6 animate-spin text-blue-600 mr-2" />
        <span>Loading plans...</span>
      </div>
    );
  }

  if (error) {
    return (
      <div className="bg-red-50 dark:bg-red-900/20 border-l-4 border-red-500 p-4 my-4">
        <div className="flex">
          <AlertCircle className="h-6 w-6 text-red-500 mr-3" />
          <div>
            <p className="text-sm text-red-800 dark:text-red-200">{error}</p>
            <button 
              className="text-sm text-red-800 dark:text-red-200 underline mt-1"
              onClick={fetchPlans}
            >
              Try again
            </button>
          </div>
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

  return (
    <div>
      {isAdmin && (
        <div className="flex justify-between items-center mb-4">
          <h2 className="text-xl font-semibold dark:text-white">Plans</h2>
          <button 
            className="flex items-center bg-blue-600 hover:bg-blue-700 text-white py-2 px-4 rounded"
            onClick={() => router.push('/dashboard/plans/create')}
          >
            <Package size={16} className="mr-2" />
            Add Plan
          </button>
        </div>
      )}
      
      <div className="overflow-x-auto bg-white dark:bg-zinc-800 rounded-lg shadow">
        <table className="min-w-full divide-y divide-gray-200 dark:divide-zinc-700">
          <thead className="bg-gray-50 dark:bg-zinc-700">
            <tr>
              <th 
                scope="col" 
                className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider cursor-pointer"
                onClick={() => handleSort('name')}
              >
                <div className="flex items-center">
                  Plan Name
                  {sortBy === 'name' && (
                    <ChevronDown 
                      size={16} 
                      className={`ml-1 ${sortDirection === 'desc' ? 'transform rotate-180' : ''}`} 
                    />
                  )}
                </div>
              </th>
              <th 
                scope="col" 
                className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider cursor-pointer"
                onClick={() => handleSort('price')}
              >
                <div className="flex items-center">
                  Price
                  {sortBy === 'price' && (
                    <ChevronDown 
                      size={16} 
                      className={`ml-1 ${sortDirection === 'desc' ? 'transform rotate-180' : ''}`} 
                    />
                  )}
                </div>
              </th>
              <th scope="col" className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                Traffic
              </th>
              <th scope="col" className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                Bandwidth
              </th>
              <th 
                scope="col" 
                className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider cursor-pointer"
                onClick={() => handleSort('billingCycle')}
              >
                <div className="flex items-center">
                  Billing Cycle
                  {sortBy === 'billingCycle' && (
                    <ChevronDown 
                      size={16} 
                      className={`ml-1 ${sortDirection === 'desc' ? 'transform rotate-180' : ''}`} 
                    />
                  )}
                </div>
              </th>
              <th scope="col" className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                Status
              </th>
              <th scope="col" className="px-6 py-3 text-right text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                Actions
              </th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-200 dark:divide-zinc-700">
            {sortedPlans.length === 0 ? (
              <tr>
                <td colSpan={7} className="px-6 py-4 text-center text-gray-500 dark:text-gray-400">
                  No plans found
                </td>
              </tr>
            ) : (
              sortedPlans.map((plan) => (
                <tr key={plan.id} className="hover:bg-gray-50 dark:hover:bg-zinc-700">
                  <td className="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900 dark:text-white">
                    {plan.name}
                    {plan.description && (
                      <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">{plan.description}</p>
                    )}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-300">
                    ${plan.price.toFixed(2)}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-300">
                    {plan.maxTraffic === 0 ? 'Unlimited' : formatBytes(plan.maxTraffic)}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-300">
                    {formatBandwidth(plan.maxBandwidth)}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-300">
                    <span className="capitalize">{plan.billingCycle}</span>
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-300">
                    <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${
                      plan.enabled ? 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-300' : 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-300'
                    }`}>
                      {plan.enabled ? 'Active' : 'Disabled'}
                    </span>
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-right text-sm font-medium">
                    <div className="flex justify-end space-x-2">
                      {isAdmin ? (
                        <>
                          <button
                            onClick={() => togglePlanStatus(plan)}
                            disabled={actionInProgress[plan.id]}
                            className={`p-1 rounded-md ${plan.enabled ? 'bg-red-100 text-red-600 hover:bg-red-200 dark:bg-red-900/50 dark:text-red-300 dark:hover:bg-red-800' : 'bg-green-100 text-green-600 hover:bg-green-200 dark:bg-green-900/50 dark:text-green-300 dark:hover:bg-green-800'}`}
                            title={plan.enabled ? 'Disable plan' : 'Enable plan'}
                          >
                            {actionInProgress[plan.id] ? (
                              <Loader2 size={16} className="animate-spin" />
                            ) : plan.enabled ? (
                              <X size={16} />
                            ) : (
                              <Check size={16} />
                            )}
                          </button>
                          <button
                            onClick={() => router.push(`/dashboard/plans/${plan.id}/edit`)}
                            className="p-1 rounded-md bg-blue-100 text-blue-600 hover:bg-blue-200 dark:bg-blue-900/50 dark:text-blue-300 dark:hover:bg-blue-800"
                            title="Edit plan"
                          >
                            <Pencil size={16} />
                          </button>
                          <button
                            onClick={() => deletePlan(plan)}
                            disabled={actionInProgress[plan.id]}
                            className="p-1 rounded-md bg-red-100 text-red-600 hover:bg-red-200 dark:bg-red-900/50 dark:text-red-300 dark:hover:bg-red-800"
                            title="Delete plan"
                          >
                            {actionInProgress[plan.id] ? (
                              <Loader2 size={16} className="animate-spin" />
                            ) : (
                              <Trash size={16} />
                            )}
                          </button>
                        </>
                      ) : (
                        <button
                          onClick={() => subscribeToPlan(plan)}
                          className="py-1 px-3 rounded-md bg-blue-600 text-white hover:bg-blue-700"
                          disabled={!plan.enabled}
                        >
                          Subscribe
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
};

export default PlansTable;
