"use client";

import React, { useState, useEffect } from 'react';
import PlansTable from '@/components/plans/plans-table';
import SubscriptionCard from '@/components/plans/subscription-card';
import { PlansService } from '@/lib/api/plans';
import { Subscription } from '@/lib/api/types';
import { Loader2 } from 'lucide-react';

export default function PlansPage() {
  const [loading, setLoading] = useState(true);
  const [subscription, setSubscription] = useState<Subscription | null>(null);
  const [isAdmin, setIsAdmin] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const fetchData = async () => {
      try {
        setLoading(true);
        const plansService = new PlansService();
        
        // Check if user is admin (could be fetched from auth context in a real app)
        // For demo purpose, let's assume we have a getCurrentUser endpoint
        const userResponse = await fetch('/api/users/me');
        const userData = await userResponse.json();
        setIsAdmin(userData.data?.role === 'admin');
        
        // Get current subscription
        const subResponse = await plansService.getCurrentSubscription();
        if (subResponse.success && subResponse.data) {
          setSubscription(subResponse.data);
        }
      } catch (err) {
        console.error("Error fetching data:", err);
        setError("Failed to load subscription data");
      } finally {
        setLoading(false);
      }
    };
    
    fetchData();
  }, []);

  if (loading) {
    return (
      <div className="container mx-auto px-4 py-6 flex justify-center items-center min-h-[300px]">
        <Loader2 className="h-8 w-8 animate-spin text-blue-600 mr-2" />
        <span className="text-lg">Loading plans data...</span>
      </div>
    );
  }

  return (
    <div className="container mx-auto px-4 py-6">
      <h1 className="text-2xl font-bold mb-6 dark:text-white">
        {isAdmin ? "Plan Management" : "Subscription Plans"}
      </h1>
      
      {!isAdmin && (
        <div className="mb-8">
          <h2 className="text-xl font-medium mb-4 dark:text-white">Your Subscription</h2>
          <div className="max-w-md">
            <SubscriptionCard subscription={subscription || undefined} />
          </div>
        </div>
      )}
      
      <PlansTable isAdmin={isAdmin} />
    </div>
  );
}
