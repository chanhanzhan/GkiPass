"use client";

import React from 'react';
import { ArrowLeft } from 'lucide-react';
import { useRouter } from 'next/navigation';
import PlanForm from '@/components/plans/plan-form';
import { ProtectedRoute } from '@/lib/auth/protected-route';

interface EditPlanPageProps {
  params: {
    id: string;
  };
}

export default function EditPlanPage({ params }: EditPlanPageProps) {
  const router = useRouter();
  const planId = params.id;
  
  return (
    <ProtectedRoute adminOnly>
      <div className="container mx-auto px-4 py-6">
        <button 
          onClick={() => router.push('/dashboard/plans')}
          className="flex items-center text-blue-600 dark:text-blue-400 hover:underline mb-4"
        >
          <ArrowLeft size={16} className="mr-1" />
          Back to Plans
        </button>
        
        <h1 className="text-2xl font-bold mb-6 dark:text-white">Edit Plan</h1>
        
        <PlanForm planId={planId} />
      </div>
    </ProtectedRoute>
  );
}
