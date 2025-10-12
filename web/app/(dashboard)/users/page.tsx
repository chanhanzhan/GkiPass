"use client";

import React from 'react';
import UsersTable from '@/components/users/users-table';
import { ProtectedRoute } from '@/lib/auth/protected-route';

export default function UsersPage() {
  return (
    <ProtectedRoute adminOnly>
      <div className="container mx-auto px-4 py-6">
        <h1 className="text-2xl font-bold mb-6 dark:text-white">User Management</h1>
        <UsersTable />
      </div>
    </ProtectedRoute>
  );
}
