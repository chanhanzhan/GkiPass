"use client";

import { API_BASE_URL, DEFAULT_TIMEOUT, getDefaultHeaders } from './config';
import { ApiResponse } from './types';
import { getProxiedApiUrl } from './cors-proxy';

/**
 * Base API client for making HTTP requests to the backend
 */
export class ApiClient {
  /**
   * Make a GET request to the API
   */
  static async get<T>(endpoint: string, params?: Record<string, any>): Promise<ApiResponse<T>> {
    const url = new URL(getProxiedApiUrl(endpoint));
    
    // Add query parameters if provided
    if (params) {
      Object.entries(params).forEach(([key, value]) => {
        if (value !== undefined && value !== null) {
          url.searchParams.append(key, String(value));
        }
      });
    }
    
    const response = await fetch(url.toString(), {
      method: 'GET',
      headers: getDefaultHeaders(),
      // 临时移除credentials模式，使用无凭证请求
      // credentials: 'include',
      next: { revalidate: 60 }, // Cache for 60 seconds
    });
    
    return this.handleResponse<T>(response);
  }
  
  /**
   * Make a POST request to the API
   */
  static async post<T>(endpoint: string, data?: any): Promise<ApiResponse<T>> {
    const response = await fetch(getProxiedApiUrl(endpoint), {
      method: 'POST',
      headers: getDefaultHeaders(),
      body: data ? JSON.stringify(data) : undefined,
      // 临时移除credentials模式，使用无凭证请求
      // credentials: 'include',
    });
    
    return this.handleResponse<T>(response);
  }
  
  /**
   * Make a PUT request to the API
   */
  static async put<T>(endpoint: string, data?: any): Promise<ApiResponse<T>> {
    const response = await fetch(getProxiedApiUrl(endpoint), {
      method: 'PUT',
      headers: getDefaultHeaders(),
      body: data ? JSON.stringify(data) : undefined,
      // 临时移除credentials模式，使用无凭证请求
      // credentials: 'include',
    });
    
    return this.handleResponse<T>(response);
  }
  
  /**
   * Make a DELETE request to the API
   */
  static async delete<T>(endpoint: string): Promise<ApiResponse<T>> {
    const response = await fetch(getProxiedApiUrl(endpoint), {
      method: 'DELETE',
      headers: getDefaultHeaders(),
      // 临时移除credentials模式，使用无凭证请求
      // credentials: 'include',
    });
    
    return this.handleResponse<T>(response);
  }
  
  /**
   * Handle API response and parse JSON data
   */
  private static async handleResponse<T>(response: Response): Promise<ApiResponse<T>> {
    // For non-JSON responses
    if (!response.headers.get('content-type')?.includes('application/json')) {
      if (!response.ok) {
        throw new Error(`API error: ${response.status} ${response.statusText}`);
      }
      return {
        success: true,
        code: response.status,
        timestamp: Date.now(),
      };
    }
    
    // For JSON responses
    const data = await response.json();
    
    // Handle API error responses
    if (!response.ok) {
      throw new Error(data.error || data.message || `API error: ${response.status}`);
    }
    
    return data as ApiResponse<T>;
  }
}