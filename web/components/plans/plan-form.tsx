"use client";

import React, { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { Loader2 } from 'lucide-react';
import { Plan } from '@/lib/api/types';
import { PlansService } from '@/lib/api/plans';

interface PlanFormProps {
  planId?: string;
  initialData?: Partial<Plan>;
}

const PlanForm: React.FC<PlanFormProps> = ({ planId, initialData = {} }) => {
  const router = useRouter();
  const isEditing = !!planId;
  
  const [formData, setFormData] = useState({
    name: '',
    description: '',
    maxRules: 0,
    maxTraffic: 0,
    maxBandwidth: 0,
    maxConnections: 0,
    billingCycle: 'monthly',
    price: 0,
    enabled: true,
    ...initialData
  });
  
  const [loading, setLoading] = useState(isEditing && !initialData.id);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);
  
  useEffect(() => {
    const fetchPlan = async () => {
      if (!isEditing) return;
      
      try {
        setLoading(true);
        const plansService = new PlansService();
        const response = await plansService.getPlan(planId!);
        
        if (response.success && response.data) {
          setFormData({
            ...formData,
            ...response.data
          });
        } else {
          setError(response.message || "Failed to fetch plan");
        }
      } catch (err) {
        setError("An error occurred while fetching plan data");
        console.error(err);
      } finally {
        setLoading(false);
      }
    };
    
    if (isEditing && !initialData.id) {
      fetchPlan();
    }
  }, [planId, isEditing, initialData]);
  
  const handleChange = (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement>) => {
    const { name, value, type } = e.target as HTMLInputElement;
    
    let parsedValue: string | number | boolean = value;
    
    if (type === 'checkbox') {
      parsedValue = (e.target as HTMLInputElement).checked;
    } else if (type === 'number') {
      parsedValue = value === '' ? 0 : Number(value);
    }
    
    setFormData({
      ...formData,
      [name]: parsedValue
    });
  };
  
  const validateForm = () => {
    // Reset errors
    setError(null);
    
    // Validate name
    if (!formData.name || formData.name.trim().length < 3) {
      setError("Plan name must be at least 3 characters long");
      return false;
    }
    
    // Validate price
    if (formData.price < 0) {
      setError("Price must be a positive number");
      return false;
    }
    
    return true;
  };
  
  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    
    if (!validateForm()) return;
    
    try {
      setSubmitting(true);
      setError(null);
      setSuccessMessage(null);
      
      const plansService = new PlansService();
      
      const planData = {
        name: formData.name,
        description: formData.description,
        maxRules: formData.maxRules,
        maxTraffic: formData.maxTraffic,
        maxBandwidth: formData.maxBandwidth,
        maxConnections: formData.maxConnections,
        billingCycle: formData.billingCycle,
        price: formData.price,
        enabled: formData.enabled
      };
      
      const response = isEditing
        ? await plansService.updatePlan(planId!, planData)
        : await plansService.createPlan(planData);
      
      if (response.success) {
        setSuccessMessage(isEditing ? "Plan updated successfully" : "Plan created successfully");
        
        // Redirect after a short delay
        setTimeout(() => {
          router.push('/dashboard/plans');
        }, 1500);
      } else {
        setError(response.message || "Failed to save plan");
      }
    } catch (err) {
      setError("An error occurred while saving plan");
      console.error(err);
    } finally {
      setSubmitting(false);
    }
  };
  
  const handleCancel = () => {
    router.push('/dashboard/plans');
  };
  
  if (loading) {
    return (
      <div className="flex justify-center items-center h-64">
        <Loader2 className="h-8 w-8 animate-spin text-blue-600" />
        <span className="ml-2 text-gray-700 dark:text-gray-300">Loading plan data...</span>
      </div>
    );
  }
  
  return (
    <div className="bg-white dark:bg-zinc-800 shadow rounded-lg p-6">
      <h2 className="text-lg font-medium mb-6 dark:text-white">
        {isEditing ? 'Edit Plan' : 'Create New Plan'}
      </h2>
      
      {error && (
        <div className="bg-red-50 dark:bg-red-900/20 text-red-800 dark:text-red-200 p-3 rounded mb-4">
          {error}
        </div>
      )}
      
      {successMessage && (
        <div className="bg-green-50 dark:bg-green-900/20 text-green-800 dark:text-green-200 p-3 rounded mb-4">
          {successMessage}
        </div>
      )}
      
      <form onSubmit={handleSubmit}>
        <div className="grid grid-cols-1 gap-y-6 gap-x-4 sm:grid-cols-2">
          <div className="sm:col-span-2">
            <label htmlFor="name" className="block text-sm font-medium text-gray-700 dark:text-gray-300">
              Plan Name
            </label>
            <input
              type="text"
              name="name"
              id="name"
              value={formData.name}
              onChange={handleChange}
              required
              className="mt-1 block w-full border-gray-300 dark:border-zinc-700 dark:bg-zinc-900 dark:text-white rounded-md shadow-sm focus:ring-blue-500 focus:border-blue-500"
            />
          </div>
          
          <div className="sm:col-span-2">
            <label htmlFor="description" className="block text-sm font-medium text-gray-700 dark:text-gray-300">
              Description
            </label>
            <textarea
              name="description"
              id="description"
              value={formData.description}
              onChange={handleChange}
              rows={3}
              className="mt-1 block w-full border-gray-300 dark:border-zinc-700 dark:bg-zinc-900 dark:text-white rounded-md shadow-sm focus:ring-blue-500 focus:border-blue-500"
            />
          </div>
          
          <div>
            <label htmlFor="price" className="block text-sm font-medium text-gray-700 dark:text-gray-300">
              Price
            </label>
            <div className="mt-1 relative rounded-md shadow-sm">
              <div className="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
                <span className="text-gray-500 sm:text-sm">$</span>
              </div>
              <input
                type="number"
                name="price"
                id="price"
                min="0"
                step="0.01"
                value={formData.price}
                onChange={handleChange}
                className="pl-7 block w-full border-gray-300 dark:border-zinc-700 dark:bg-zinc-900 dark:text-white rounded-md shadow-sm focus:ring-blue-500 focus:border-blue-500"
              />
            </div>
          </div>
          
          <div>
            <label htmlFor="billingCycle" className="block text-sm font-medium text-gray-700 dark:text-gray-300">
              Billing Cycle
            </label>
            <select
              id="billingCycle"
              name="billingCycle"
              value={formData.billingCycle}
              onChange={handleChange}
              className="mt-1 block w-full border-gray-300 dark:border-zinc-700 dark:bg-zinc-900 dark:text-white rounded-md shadow-sm focus:ring-blue-500 focus:border-blue-500"
            >
              <option value="monthly">Monthly</option>
              <option value="yearly">Yearly</option>
              <option value="permanent">Permanent</option>
            </select>
          </div>
          
          <div>
            <label htmlFor="maxTraffic" className="block text-sm font-medium text-gray-700 dark:text-gray-300">
              Max Traffic (GB, 0 for unlimited)
            </label>
            <input
              type="number"
              name="maxTraffic"
              id="maxTraffic"
              min="0"
              value={formData.maxTraffic}
              onChange={handleChange}
              className="mt-1 block w-full border-gray-300 dark:border-zinc-700 dark:bg-zinc-900 dark:text-white rounded-md shadow-sm focus:ring-blue-500 focus:border-blue-500"
            />
          </div>
          
          <div>
            <label htmlFor="maxBandwidth" className="block text-sm font-medium text-gray-700 dark:text-gray-300">
              Max Bandwidth (bps, 0 for unlimited)
            </label>
            <input
              type="number"
              name="maxBandwidth"
              id="maxBandwidth"
              min="0"
              value={formData.maxBandwidth}
              onChange={handleChange}
              className="mt-1 block w-full border-gray-300 dark:border-zinc-700 dark:bg-zinc-900 dark:text-white rounded-md shadow-sm focus:ring-blue-500 focus:border-blue-500"
            />
          </div>
          
          <div>
            <label htmlFor="maxRules" className="block text-sm font-medium text-gray-700 dark:text-gray-300">
              Max Rules (0 for unlimited)
            </label>
            <input
              type="number"
              name="maxRules"
              id="maxRules"
              min="0"
              value={formData.maxRules}
              onChange={handleChange}
              className="mt-1 block w-full border-gray-300 dark:border-zinc-700 dark:bg-zinc-900 dark:text-white rounded-md shadow-sm focus:ring-blue-500 focus:border-blue-500"
            />
          </div>
          
          <div>
            <label htmlFor="maxConnections" className="block text-sm font-medium text-gray-700 dark:text-gray-300">
              Max Connections (0 for unlimited)
            </label>
            <input
              type="number"
              name="maxConnections"
              id="maxConnections"
              min="0"
              value={formData.maxConnections}
              onChange={handleChange}
              className="mt-1 block w-full border-gray-300 dark:border-zinc-700 dark:bg-zinc-900 dark:text-white rounded-md shadow-sm focus:ring-blue-500 focus:border-blue-500"
            />
          </div>
          
          <div className="flex items-center h-full pt-5">
            <input
              type="checkbox"
              id="enabled"
              name="enabled"
              checked={formData.enabled}
              onChange={handleChange}
              className="h-4 w-4 text-blue-600 focus:ring-blue-500 border-gray-300 rounded"
            />
            <label htmlFor="enabled" className="ml-2 block text-sm text-gray-700 dark:text-gray-300">
              Enable this plan
            </label>
          </div>
        </div>
        
        <div className="mt-8 flex justify-end">
          <button
            type="button"
            onClick={handleCancel}
            className="bg-white dark:bg-zinc-700 py-2 px-4 border border-gray-300 dark:border-zinc-600 rounded-md shadow-sm text-sm font-medium text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-zinc-600 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={submitting}
            className="ml-3 inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-medium rounded-md text-white bg-blue-600 hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {submitting ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin mr-2" />
                {isEditing ? 'Updating...' : 'Creating...'}
              </>
            ) : (
              <>{isEditing ? 'Update' : 'Create'}</>
            )}
          </button>
        </div>
      </form>
    </div>
  );
};

export default PlanForm;
