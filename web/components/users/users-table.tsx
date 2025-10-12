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
  UserPlus,
  Loader2 
} from 'lucide-react';
import { User } from '@/lib/api/types';
import { userService } from '@/lib/api/users';

interface UsersTableProps {
  initialUsers?: User[];
}

const UsersTable: React.FC<UsersTableProps> = ({ initialUsers = [] }) => {
  const router = useRouter();
  const [users, setUsers] = useState<User[]>(initialUsers);
  const [loading, setLoading] = useState<boolean>(initialUsers.length === 0);
  const [error, setError] = useState<string | null>(null);
  const [sortBy, setSortBy] = useState<string>('username');
  const [sortDirection, setSortDirection] = useState<'asc' | 'desc'>('asc');
  const [actionInProgress, setActionInProgress] = useState<{[key: string]: boolean}>({});

  const fetchUsers = async () => {
    try {
      setLoading(true);
      setError(null);
      // Use the imported userService singleton
      const response = await userService.getUsers();
      if (response.success && response.data) {
        setUsers(response.data);
      } else {
        setError(response.message || "Failed to fetch users");
      }
    } catch (err) {
      setError("An error occurred while fetching users");
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (initialUsers.length === 0) {
      fetchUsers();
    }
  }, [initialUsers.length]);

  const handleSort = (field: string) => {
    if (sortBy === field) {
      setSortDirection(sortDirection === 'asc' ? 'desc' : 'asc');
    } else {
      setSortBy(field);
      setSortDirection('asc');
    }
  };

  const sortedUsers = [...users].sort((a, b) => {
    let comparison = 0;
    
    if (sortBy === 'username') {
      comparison = a.username.localeCompare(b.username);
    } else if (sortBy === 'email') {
      comparison = a.email.localeCompare(b.email);
    } else if (sortBy === 'role') {
      comparison = a.role.localeCompare(b.role);
    } else if (sortBy === 'createdAt') {
      comparison = new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime();
    }
    
    return sortDirection === 'asc' ? comparison : -comparison;
  });

  const toggleUserStatus = async (user: User) => {
    try {
      setActionInProgress({ ...actionInProgress, [user.id]: true });
      // Use the imported userService singleton
      const response = await userService.toggleUserStatus(user.id, !user.enabled);
      
      if (response.success) {
        setUsers(users.map(u => u.id === user.id ? { ...u, enabled: !u.enabled } : u));
      } else {
        setError(`Failed to update user status: ${response.message}`);
      }
    } catch (err) {
      setError("An error occurred while updating user status");
      console.error(err);
    } finally {
      setActionInProgress({ ...actionInProgress, [user.id]: false });
    }
  };

  const deleteUser = async (user: User) => {
    if (!window.confirm(`Are you sure you want to delete user ${user.username}?`)) {
      return;
    }

    try {
      setActionInProgress({ ...actionInProgress, [user.id]: true });
      // Use the imported userService singleton
      const response = await userService.deleteUser(user.id);
      
      if (response.success) {
        setUsers(users.filter(u => u.id !== user.id));
      } else {
        setError(`Failed to delete user: ${response.message}`);
      }
    } catch (err) {
      setError("An error occurred while deleting user");
      console.error(err);
    } finally {
      setActionInProgress({ ...actionInProgress, [user.id]: false });
    }
  };

  if (loading) {
    return (
      <div className="flex justify-center items-center min-h-[200px]">
        <Loader2 className="h-6 w-6 animate-spin text-blue-600 mr-2" />
        <span>Loading users...</span>
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
              onClick={fetchUsers}
            >
              Try again
            </button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <h2 className="text-xl font-semibold dark:text-white">Users</h2>
        <button 
          className="flex items-center bg-blue-600 hover:bg-blue-700 text-white py-2 px-4 rounded"
          onClick={() => router.push('/dashboard/users/create')}
        >
          <UserPlus size={16} className="mr-2" />
          Add User
        </button>
      </div>
      
      <div className="overflow-x-auto bg-white dark:bg-zinc-800 rounded-lg shadow">
        <table className="min-w-full divide-y divide-gray-200 dark:divide-zinc-700">
          <thead className="bg-gray-50 dark:bg-zinc-700">
            <tr>
              <th 
                scope="col" 
                className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider cursor-pointer"
                onClick={() => handleSort('username')}
              >
                <div className="flex items-center">
                  Username
                  {sortBy === 'username' && (
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
                onClick={() => handleSort('email')}
              >
                <div className="flex items-center">
                  Email
                  {sortBy === 'email' && (
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
                onClick={() => handleSort('role')}
              >
                <div className="flex items-center">
                  Role
                  {sortBy === 'role' && (
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
              <th 
                scope="col" 
                className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider cursor-pointer"
                onClick={() => handleSort('createdAt')}
              >
                <div className="flex items-center">
                  Created
                  {sortBy === 'createdAt' && (
                    <ChevronDown 
                      size={16} 
                      className={`ml-1 ${sortDirection === 'desc' ? 'transform rotate-180' : ''}`} 
                    />
                  )}
                </div>
              </th>
              <th scope="col" className="px-6 py-3 text-right text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                Actions
              </th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-200 dark:divide-zinc-700">
            {sortedUsers.length === 0 ? (
              <tr>
                <td colSpan={6} className="px-6 py-4 text-center text-gray-500 dark:text-gray-400">
                  No users found
                </td>
              </tr>
            ) : (
              sortedUsers.map((user) => (
                <tr key={user.id} className="hover:bg-gray-50 dark:hover:bg-zinc-700">
                  <td className="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900 dark:text-white">
                    {user.username}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-300">
                    {user.email}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-300">
                    <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${
                      user.role === 'admin' ? 'bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-300' : 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-300'
                    }`}>
                      {user.role}
                    </span>
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-300">
                    <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${
                      user.enabled ? 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-300' : 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-300'
                    }`}>
                      {user.enabled ? 'Active' : 'Disabled'}
                    </span>
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-300">
                    {new Date(user.createdAt).toLocaleDateString()}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-right text-sm font-medium">
                    <div className="flex justify-end space-x-2">
                      <button
                        onClick={() => toggleUserStatus(user)}
                        disabled={actionInProgress[user.id]}
                        className={`p-1 rounded-md ${user.enabled ? 'bg-red-100 text-red-600 hover:bg-red-200 dark:bg-red-900/50 dark:text-red-300 dark:hover:bg-red-800' : 'bg-green-100 text-green-600 hover:bg-green-200 dark:bg-green-900/50 dark:text-green-300 dark:hover:bg-green-800'}`}
                        title={user.enabled ? 'Disable user' : 'Enable user'}
                      >
                        {actionInProgress[user.id] ? (
                          <Loader2 size={16} className="animate-spin" />
                        ) : user.enabled ? (
                          <X size={16} />
                        ) : (
                          <Check size={16} />
                        )}
                      </button>
                      <button
                        onClick={() => router.push(`/dashboard/users/${user.id}/edit`)}
                        className="p-1 rounded-md bg-blue-100 text-blue-600 hover:bg-blue-200 dark:bg-blue-900/50 dark:text-blue-300 dark:hover:bg-blue-800"
                        title="Edit user"
                      >
                        <Pencil size={16} />
                      </button>
                      <button
                        onClick={() => deleteUser(user)}
                        disabled={actionInProgress[user.id]}
                        className="p-1 rounded-md bg-red-100 text-red-600 hover:bg-red-200 dark:bg-red-900/50 dark:text-red-300 dark:hover:bg-red-800"
                        title="Delete user"
                      >
                        {actionInProgress[user.id] ? (
                          <Loader2 size={16} className="animate-spin" />
                        ) : (
                          <Trash size={16} />
                        )}
                      </button>
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

export default UsersTable;
