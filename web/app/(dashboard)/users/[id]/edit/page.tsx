"use client";

import React from 'react';
import { ArrowLeft } from 'lucide-react';
import { useRouter } from 'next/navigation';
import UserForm from '@/components/users/user-form';
import { ProtectedRoute } from '@/lib/auth/protected-route';

interface EditUserPageProps {
  params: {
    id: string;
  };
}

export default function EditUserPage({ params }: EditUserPageProps) {
  const router = useRouter();
  const userId = params.id;
  
  return (
    <ProtectedRoute adminOnly>
      <div className="container mx-auto px-4 py-6">
        <button 
          onClick={() => router.push('/dashboard/users')}
          className="flex items-center text-blue-600 dark:text-blue-400 hover:underline mb-4"
        >
          <ArrowLeft size={16} className="mr-1" />
          Back to Users
        </button>
        
        <h1 className="text-2xl font-bold mb-6 dark:text-white">Edit User</h1>
        
        <UserForm userId={userId} />
      </div>
    </ProtectedRoute>
  );
}
