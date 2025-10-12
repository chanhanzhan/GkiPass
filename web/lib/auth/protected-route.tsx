"use client";

import React, { useEffect } from 'react';
import { useRouter, usePathname } from 'next/navigation';
import { useAuth } from './auth-context';
import { Loader2 } from 'lucide-react';

interface ProtectedRouteProps {
  children: React.ReactNode;
  adminOnly?: boolean;
}

export const ProtectedRoute: React.FC<ProtectedRouteProps> = ({ 
  children, 
  adminOnly = false 
}) => {
  const { user, isLoading, isAdmin } = useAuth();
  const router = useRouter();
  const pathname = usePathname();

  useEffect(() => {
    // Skip redirects during loading
    if (isLoading) return;
    
    // If no user is logged in, redirect to login
    if (!user) {
      // Store the current path to redirect back after login
      sessionStorage.setItem('redirect_after_login', pathname);
      router.push('/login');
      return;
    }
    
    // If route requires admin but user is not admin
    if (adminOnly && !isAdmin) {
      // Redirect to dashboard
      router.push('/dashboard/dashboard');
    }
  }, [user, isLoading, isAdmin, router, pathname, adminOnly]);

  if (isLoading) {
    return (
      <div className="flex justify-center items-center min-h-screen">
        <div className="text-center">
          <Loader2 className="h-8 w-8 animate-spin mx-auto mb-4 text-blue-600" />
          <p className="text-gray-600 dark:text-gray-400">Loading...</p>
        </div>
      </div>
    );
  }

  // If user is logged in and has proper permissions, render children
  if (user && (!adminOnly || isAdmin)) {
    return <>{children}</>;
  }
  
  // Default fallback during redirects
  return (
    <div className="flex justify-center items-center min-h-screen">
      <div className="text-center">
        <Loader2 className="h-8 w-8 animate-spin mx-auto mb-4 text-blue-600" />
        <p className="text-gray-600 dark:text-gray-400">Redirecting...</p>
      </div>
    </div>
  );
};
