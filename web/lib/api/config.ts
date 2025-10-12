"use client";

/**
 * API Configuration
 * 
 * This module provides API configuration settings for the application.
 * It reads from environment variables when available and falls back to defaults.
 */

// Base API URL for backend services
export const API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL || 'http://localhost:8080/api/v1';

// WebSocket URLs for real-time communication
export const WS_NODE_URL = process.env.NEXT_PUBLIC_WS_NODE_URL || 'ws://localhost:8080/api/v1/ws/node';
export const WS_ADMIN_URL = process.env.NEXT_PUBLIC_WS_ADMIN_URL || 'ws://localhost:8080/api/v1/ws/admin';

// API timeouts (in milliseconds)
export const DEFAULT_TIMEOUT = 10000; // 10 seconds
export const LONG_TIMEOUT = 30000;    // 30 seconds

// Authentication token storage key
export const AUTH_TOKEN_KEY = 'gkipass_auth_token';
export const REFRESH_TOKEN_KEY = 'gkipass_refresh_token';

/**
 * Get the stored authentication token
 */
export const getAuthToken = (): string | null => {
  if (typeof window !== 'undefined') {
    return localStorage.getItem(AUTH_TOKEN_KEY);
  }
  return null;
};

/**
 * Store the authentication token
 */
export const setAuthToken = (token: string): void => {
  if (typeof window !== 'undefined') {
    localStorage.setItem(AUTH_TOKEN_KEY, token);
  }
};

/**
 * Remove the authentication token (used during logout)
 */
export const removeAuthToken = (): void => {
  if (typeof window !== 'undefined') {
    localStorage.removeItem(AUTH_TOKEN_KEY);
  }
};

/**
 * Default headers for API requests
 */
export const getDefaultHeaders = (): Record<string, string> => {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  };
  
  const token = getAuthToken();
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }
  
  return headers;
};
