"use client";

import { ApiClient } from './client';
import { ApiResponse, CaptchaResponse, LoginRequest, LoginResponse, User } from './types';
import { removeAuthToken, setAuthToken } from './config';

/**
 * Authentication API service
 */
class AuthService {
  /**
   * User login
   */
  async login(data: LoginRequest): Promise<ApiResponse<LoginResponse>> {
    const response = await ApiClient.post<LoginResponse>('/auth/login', {
      username: data.username,
      password: data.password,
      captcha_id: data.captcha_id,
      captcha_code: data.captcha_code
    });
    
    if (response.success && response.data?.token) {
      setAuthToken(response.data.token);
    }
    
    return response;
  }
  
  /**
   * User logout
   */
  async logout(): Promise<ApiResponse<void>> {
    const response = await ApiClient.post<void>('/auth/logout');
    
    // Always remove the token, even if the API call fails
    removeAuthToken();
    
    return response;
  }
  
  /**
   * Register new user
   */
  async register(data: { 
    username: string; 
    email: string; 
    password: string; 
    captcha_id?: string;
    captcha_code?: string;
  }): Promise<ApiResponse<User>> {
    return ApiClient.post<User>('/auth/register', data);
  }

  /**
   * Get current user profile
   */
  async getCurrentUser(): Promise<ApiResponse<User>> {
    return ApiClient.get<User>('/users/me');
  }
  
  /**
   * Refresh authentication token
   */
  async refreshToken(): Promise<ApiResponse<LoginResponse>> {
    const response = await ApiClient.post<LoginResponse>('/auth/refresh');
    
    if (response.success && response.data?.token) {
      setAuthToken(response.data.token);
    }
    
    return response;
  }
  
  /**
   * Get captcha image for verification
   */
  async getCaptcha(): Promise<ApiResponse<CaptchaResponse>> {
    return ApiClient.get<CaptchaResponse>('/auth/captcha');
  }
  
  /**
   * Verify captcha code
   */
  async verifyCaptcha(id: string, code: string): Promise<ApiResponse<boolean>> {
    return ApiClient.post<boolean>('/auth/captcha/verify', {
      captcha_id: id,
      code: code
    });
  }
  
  /**
   * Check if user is authenticated
   */
  isAuthenticated(): boolean {
    if (typeof window === 'undefined') {
      return false;
    }
    
    const token = localStorage.getItem('gkipass_auth_token');
    return !!token;
  }
}

// Export a singleton instance
export const authService = new AuthService();