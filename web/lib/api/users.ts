"use client";

import { ApiClient } from './client';
import { ApiResponse, PaginationParams, User } from './types';

/**
 * Users API service
 */
class UserService {
  /**
   * Get a list of users
   */
  async getUsers(params?: {
    role?: string;
    enabled?: boolean;
  } & PaginationParams): Promise<ApiResponse<User[]>> {
    return ApiClient.get<User[]>('/users', params);
  }
  
  /**
   * Get a single user by ID
   */
  async getUser(id: string): Promise<ApiResponse<User>> {
    return ApiClient.get<User>(`/users/${id}`);
  }
  
  /**
   * Get current user profile
   */
  async getCurrentUser(): Promise<ApiResponse<User>> {
    return ApiClient.get<User>('/users/me');
  }
  
  /**
   * Create a new user
   */
  async createUser(user: {
    username: string;
    password: string;
    email: string;
    role?: string;
    enabled?: boolean;
  }): Promise<ApiResponse<User>> {
    return ApiClient.post<User>('/users', user);
  }
  
  /**
   * Update an existing user
   */
  async updateUser(id: string, user: {
    username?: string;
    email?: string;
    role?: string;
    enabled?: boolean;
    avatar?: string;
  }): Promise<ApiResponse<User>> {
    return ApiClient.put<User>(`/users/${id}`, user);
  }
  
  /**
   * Delete a user
   */
  async deleteUser(id: string): Promise<ApiResponse<void>> {
    return ApiClient.delete<void>(`/users/${id}`);
  }
  
  /**
   * Change user password
   */
  async changePassword(id: string, data: {
    old_password: string;
    new_password: string;
  }): Promise<ApiResponse<void>> {
    return ApiClient.put<void>(`/users/${id}/password`, data);
  }
  
  /**
   * Reset user password (admin only)
   */
  async resetPassword(id: string, newPassword: string): Promise<ApiResponse<void>> {
    return ApiClient.post<void>(`/users/${id}/password/reset`, { 
      password: newPassword 
    });
  }
  
  /**
   * Enable user account
   */
  async enableUser(id: string): Promise<ApiResponse<User>> {
    return ApiClient.put<User>(`/users/${id}`, { enabled: true });
  }
  
  /**
   * Disable user account
   */
  async disableUser(id: string): Promise<ApiResponse<User>> {
    return ApiClient.put<User>(`/users/${id}`, { enabled: false });
  }
  
  /**
   * Update user role (admin only)
   */
  async updateUserRole(id: string, role: string): Promise<ApiResponse<User>> {
    return ApiClient.put<User>(`/users/${id}/role`, { role });
  }
  
  /**
   * Register a new user
   */
  async register(data: {
    username: string;
    password: string;
    email: string;
    captcha_id?: string;
    captcha_code?: string;
  }): Promise<ApiResponse<User>> {
    return ApiClient.post<User>('/users/register', data);
  }
  
  /**
   * Get user permissions
   */
  async getUserPermissions(id: string): Promise<ApiResponse<string[]>> {
    return ApiClient.get<string[]>(`/users/${id}/permissions`);
  }
}

// Export a singleton instance
export const userService = new UserService();