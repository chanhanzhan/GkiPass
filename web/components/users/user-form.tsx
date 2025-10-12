"use client";

import React, { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { Loader2 } from 'lucide-react';
import { User } from '@/lib/api/types';
import { userService } from '@/lib/api/users';

interface UserFormProps {
  userId?: string;
  initialData?: Partial<User>;
}

const UserForm: React.FC<UserFormProps> = ({ userId, initialData = {} }) => {
  const router = useRouter();
  const isEditing = !!userId;
  
  const [formData, setFormData] = useState({
    username: '',
    email: '',
    password: '',
    confirmPassword: '',
    role: 'user',
    enabled: true,
    ...initialData
  });
  
  const [loading, setLoading] = useState(isEditing && !initialData.id);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);
  
  useEffect(() => {
    const fetchUser = async () => {
      if (!isEditing) return;
      
      try {
        setLoading(true);
        // Use the imported userService singleton
        const response = await userService.getUser(userId!);
        
        if (response.success && response.data) {
          setFormData({
            ...formData,
            username: response.data.username,
            email: response.data.email,
            role: response.data.role,
            enabled: response.data.enabled
          });
        } else {
          setError(response.message || "Failed to fetch user");
        }
      } catch (err) {
        setError("An error occurred while fetching user data");
        console.error(err);
      } finally {
        setLoading(false);
      }
    };
    
    if (isEditing && !initialData.id) {
      fetchUser();
    }
  }, [userId, isEditing, initialData]);
  
  const handleChange = (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
    const { name, value, type } = e.target as HTMLInputElement;
    
    setFormData({
      ...formData,
      [name]: type === 'checkbox' ? (e.target as HTMLInputElement).checked : value
    });
  };
  
  const validateForm = () => {
    // Reset errors
    setError(null);
    
    // Validate username
    if (!formData.username || formData.username.trim().length < 3) {
      setError("Username must be at least 3 characters long");
      return false;
    }
    
    // Validate email
    const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
    if (!formData.email || !emailRegex.test(formData.email)) {
      setError("Please enter a valid email address");
      return false;
    }
    
    // Validate password for new users
    if (!isEditing) {
      if (!formData.password || formData.password.length < 8) {
        setError("Password must be at least 8 characters long");
        return false;
      }
      
      if (formData.password !== formData.confirmPassword) {
        setError("Passwords do not match");
        return false;
      }
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
      
      // Use the imported userService singleton
      const userData = {
        username: formData.username,
        email: formData.email,
        role: formData.role,
        enabled: formData.enabled,
        ...(isEditing ? {} : { password: formData.password })
      };
      
      const response = isEditing
        ? await userService.updateUser(userId!, userData)
        : await userService.createUser(userData);
      
      if (response.success) {
        setSuccessMessage(isEditing ? "User updated successfully" : "User created successfully");
        
        // Redirect after a short delay
        setTimeout(() => {
          router.push('/dashboard/users');
        }, 1500);
      } else {
        setError(response.message || "Failed to save user");
      }
    } catch (err) {
      setError("An error occurred while saving user");
      console.error(err);
    } finally {
      setSubmitting(false);
    }
  };
  
  const handleCancel = () => {
    router.push('/dashboard/users');
  };
  
  if (loading) {
    return (
      <div className="flex justify-center items-center h-64">
        <Loader2 className="h-8 w-8 animate-spin text-blue-600" />
        <span className="ml-2 text-gray-700 dark:text-gray-300">Loading user data...</span>
      </div>
    );
  }
  
  return (
    <div className="bg-white dark:bg-zinc-800 shadow rounded-lg p-6">
      <h2 className="text-lg font-medium mb-6 dark:text-white">
        {isEditing ? 'Edit User' : 'Create New User'}
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
          <div>
            <label htmlFor="username" className="block text-sm font-medium text-gray-700 dark:text-gray-300">
              Username
            </label>
            <input
              type="text"
              name="username"
              id="username"
              value={formData.username}
              onChange={handleChange}
              required
              className="mt-1 block w-full border-gray-300 dark:border-zinc-700 dark:bg-zinc-900 dark:text-white rounded-md shadow-sm focus:ring-blue-500 focus:border-blue-500"
            />
          </div>
          
          <div>
            <label htmlFor="email" className="block text-sm font-medium text-gray-700 dark:text-gray-300">
              Email Address
            </label>
            <input
              type="email"
              name="email"
              id="email"
              value={formData.email}
              onChange={handleChange}
              required
              className="mt-1 block w-full border-gray-300 dark:border-zinc-700 dark:bg-zinc-900 dark:text-white rounded-md shadow-sm focus:ring-blue-500 focus:border-blue-500"
            />
          </div>
          
          {!isEditing && (
            <>
              <div>
                <label htmlFor="password" className="block text-sm font-medium text-gray-700 dark:text-gray-300">
                  Password
                </label>
                <input
                  type="password"
                  name="password"
                  id="password"
                  value={formData.password}
                  onChange={handleChange}
                  required={!isEditing}
                  className="mt-1 block w-full border-gray-300 dark:border-zinc-700 dark:bg-zinc-900 dark:text-white rounded-md shadow-sm focus:ring-blue-500 focus:border-blue-500"
                />
              </div>
              
              <div>
                <label htmlFor="confirmPassword" className="block text-sm font-medium text-gray-700 dark:text-gray-300">
                  Confirm Password
                </label>
                <input
                  type="password"
                  name="confirmPassword"
                  id="confirmPassword"
                  value={formData.confirmPassword}
                  onChange={handleChange}
                  required={!isEditing}
                  className="mt-1 block w-full border-gray-300 dark:border-zinc-700 dark:bg-zinc-900 dark:text-white rounded-md shadow-sm focus:ring-blue-500 focus:border-blue-500"
                />
              </div>
            </>
          )}
          
          <div>
            <label htmlFor="role" className="block text-sm font-medium text-gray-700 dark:text-gray-300">
              Role
            </label>
            <select
              id="role"
              name="role"
              value={formData.role}
              onChange={handleChange}
              className="mt-1 block w-full border-gray-300 dark:border-zinc-700 dark:bg-zinc-900 dark:text-white rounded-md shadow-sm focus:ring-blue-500 focus:border-blue-500"
            >
              <option value="user">User</option>
              <option value="admin">Admin</option>
            </select>
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
              Account active
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

export default UserForm;
