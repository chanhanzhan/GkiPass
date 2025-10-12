"use client";

import React, { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { ArrowLeft, Loader2, CreditCard, Wallet, CheckCircle } from 'lucide-react';
import { PlansService } from '@/lib/api/plans';
import { Plan } from '@/lib/api/types';
import SubscriptionCard from '@/components/plans/subscription-card';

interface SubscribePlanPageProps {
  params: {
    id: string;
  };
}

export default function SubscribePlanPage({ params }: SubscribePlanPageProps) {
  const router = useRouter();
  const planId = params.id;
  
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [plan, setPlan] = useState<Plan | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [paymentMethod, setPaymentMethod] = useState<string>('wallet');
  const [walletBalance, setWalletBalance] = useState<number>(0);
  const [success, setSuccess] = useState(false);
  
  useEffect(() => {
    const fetchData = async () => {
      try {
        setLoading(true);
        
        // Fetch plan data
        const plansService = new PlansService();
        const response = await plansService.getPlan(planId);
        
        if (response.success && response.data) {
          setPlan(response.data);
        } else {
          setError(response.message || "Failed to fetch plan");
          return;
        }
        
        // Fetch wallet balance
        try {
          const walletResponse = await fetch('/api/wallet/balance');
          const walletData = await walletResponse.json();
          
          if (walletData.success) {
            setWalletBalance(walletData.data.balance || 0);
          }
        } catch (walletErr) {
          console.error("Error fetching wallet balance:", walletErr);
          // Non-critical error, continue anyway
        }
      } catch (err) {
        console.error("Error fetching data:", err);
        setError("Failed to load plan data");
      } finally {
        setLoading(false);
      }
    };
    
    fetchData();
  }, [planId]);
  
  const handleSubscribe = async () => {
    if (!plan) return;
    
    try {
      setSubmitting(true);
      setError(null);
      
      const plansService = new PlansService();
      const response = await plansService.subscribeToPlan(planId, paymentMethod);
      
      if (response.success) {
        setSuccess(true);
        
        // Redirect after a delay
        setTimeout(() => {
          router.push('/dashboard/subscription');
        }, 2000);
      } else {
        setError(response.message || "Failed to subscribe to plan");
      }
    } catch (err) {
      console.error("Error during subscription:", err);
      setError("An error occurred during subscription process");
    } finally {
      setSubmitting(false);
    }
  };
  
  if (loading) {
    return (
      <div className="container mx-auto px-4 py-6 flex justify-center items-center min-h-[300px]">
        <Loader2 className="h-8 w-8 animate-spin text-blue-600 mr-2" />
        <span className="text-lg">Loading plan data...</span>
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
  
  if (!plan) {
    return (
      <div className="container mx-auto px-4 py-6">
        <button 
          onClick={() => router.push('/dashboard/plans')}
          className="flex items-center text-blue-600 dark:text-blue-400 hover:underline mb-4"
        >
          <ArrowLeft size={16} className="mr-1" />
          Back to Plans
        </button>
        
        <h1 className="text-2xl font-bold mb-4 dark:text-white">Plan Not Found</h1>
        <p className="dark:text-gray-300">The plan you're looking for doesn't exist.</p>
      </div>
    );
  }

  if (success) {
    return (
      <div className="container mx-auto px-4 py-6">
        <div className="max-w-lg mx-auto bg-white dark:bg-zinc-800 rounded-lg shadow p-8 text-center">
          <CheckCircle className="h-16 w-16 text-green-500 mx-auto mb-4" />
          <h1 className="text-2xl font-bold mb-2 dark:text-white">Subscription Successful!</h1>
          <p className="text-gray-600 dark:text-gray-400 mb-6">
            You have successfully subscribed to the {plan.name} plan. You will be redirected to your subscription page shortly.
          </p>
          <button
            onClick={() => router.push('/dashboard/subscription')}
            className="px-6 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700"
          >
            View Subscription
          </button>
        </div>
      </div>
    );
  }

  const insufficientFunds = paymentMethod === 'wallet' && walletBalance < plan.price;

  return (
    <div className="container mx-auto px-4 py-6">
      <button 
        onClick={() => router.push('/dashboard/plans')}
        className="flex items-center text-blue-600 dark:text-blue-400 hover:underline mb-4"
      >
        <ArrowLeft size={16} className="mr-1" />
        Back to Plans
      </button>
      
      <h1 className="text-2xl font-bold mb-6 dark:text-white">Subscribe to {plan.name}</h1>
      
      <div className="grid grid-cols-1 md:grid-cols-2 gap-8">
        {/* Plan Details */}
        <div>
          <h2 className="text-xl font-medium mb-4 dark:text-white">Plan Details</h2>
          <div className="max-w-sm">
            <SubscriptionCard plan={plan} />
          </div>
        </div>
        
        {/* Payment Section */}
        <div className="bg-white dark:bg-zinc-800 rounded-lg shadow p-6">
          <h2 className="text-xl font-medium mb-4 dark:text-white">Payment</h2>
          
          <div className="mb-6">
            <label className="block text-gray-700 dark:text-gray-300 text-sm font-bold mb-2">
              Payment Method
            </label>
            
            <div className="space-y-3">
              <div 
                className={`p-4 border rounded-lg cursor-pointer flex items-center ${
                  paymentMethod === 'wallet' 
                    ? 'border-blue-500 bg-blue-50 dark:bg-blue-900/20' 
                    : 'border-gray-300 dark:border-zinc-700'
                }`}
                onClick={() => setPaymentMethod('wallet')}
              >
                <div className={`w-5 h-5 rounded-full border flex items-center justify-center ${
                  paymentMethod === 'wallet' ? 'border-blue-600' : 'border-gray-400'
                }`}>
                  {paymentMethod === 'wallet' && (
                    <div className="w-3 h-3 rounded-full bg-blue-600"></div>
                  )}
                </div>
                <Wallet className="h-5 w-5 text-blue-600 mx-3" />
                <div>
                  <div className="font-medium dark:text-white">Wallet</div>
                  <div className="text-sm text-gray-500 dark:text-gray-400">
                    Balance: ${walletBalance.toFixed(2)}
                  </div>
                </div>
              </div>
              
              <div 
                className={`p-4 border rounded-lg cursor-pointer flex items-center ${
                  paymentMethod === 'card' 
                    ? 'border-blue-500 bg-blue-50 dark:bg-blue-900/20' 
                    : 'border-gray-300 dark:border-zinc-700'
                }`}
                onClick={() => setPaymentMethod('card')}
              >
                <div className={`w-5 h-5 rounded-full border flex items-center justify-center ${
                  paymentMethod === 'card' ? 'border-blue-600' : 'border-gray-400'
                }`}>
                  {paymentMethod === 'card' && (
                    <div className="w-3 h-3 rounded-full bg-blue-600"></div>
                  )}
                </div>
                <CreditCard className="h-5 w-5 text-blue-600 mx-3" />
                <div>
                  <div className="font-medium dark:text-white">Credit Card</div>
                  <div className="text-sm text-gray-500 dark:text-gray-400">
                    Secure payment via Stripe
                  </div>
                </div>
              </div>
            </div>
          </div>
          
          <div className="border-t border-gray-200 dark:border-zinc-700 pt-4 mt-6">
            <div className="flex justify-between mb-2">
              <span className="text-gray-600 dark:text-gray-400">Subtotal</span>
              <span className="dark:text-white">${plan.price.toFixed(2)}</span>
            </div>
            
            <div className="flex justify-between mb-2">
              <span className="text-gray-600 dark:text-gray-400">Tax</span>
              <span className="dark:text-white">$0.00</span>
            </div>
            
            <div className="flex justify-between font-bold mt-4 pt-4 border-t border-gray-200 dark:border-zinc-700">
              <span className="dark:text-white">Total</span>
              <span className="dark:text-white">${plan.price.toFixed(2)}</span>
            </div>
          </div>
          
          {insufficientFunds && (
            <div className="mt-4 bg-red-50 dark:bg-red-900/20 border-l-4 border-red-500 p-3 text-sm text-red-700 dark:text-red-300">
              Insufficient wallet balance. Please add funds or choose another payment method.
            </div>
          )}
          
          <div className="mt-6">
            <button
              onClick={handleSubscribe}
              disabled={submitting || insufficientFunds}
              className="w-full py-2 px-4 bg-blue-600 text-white rounded-md hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {submitting ? (
                <>
                  <Loader2 className="h-4 w-4 animate-spin inline mr-2" />
                  Processing...
                </>
              ) : (
                `Subscribe for $${plan.price.toFixed(2)}`
              )}
            </button>
            
            <div className="mt-2 text-xs text-center text-gray-500 dark:text-gray-400">
              By clicking the button above, you agree to our Terms of Service and Privacy Policy.
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
