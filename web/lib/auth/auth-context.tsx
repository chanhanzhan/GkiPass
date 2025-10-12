"use client";

import React, { createContext, useState, useContext, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { authService } from '@/lib/api/auth';
import { User } from '@/lib/api/types';

interface AuthContextType {
  user: User | null;
  isLoading: boolean;
  isAdmin: boolean;
  login: (username: string, password: string) => Promise<{ success: boolean; message?: string }>;
  logout: () => Promise<void>;
  register: (username: string, email: string, password: string) => Promise<{ success: boolean; message?: string }>;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export const AuthProvider: React.FC<{children: React.ReactNode}> = ({ children }) => {
  const [user, setUser] = useState<User | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const router = useRouter();
  
  useEffect(() => {
    // Check if user is logged in on mount
    const token = localStorage.getItem('auth_token');
    
    if (token) {
      // Fetch user data
      getCurrentUser();
    } else {
      setIsLoading(false);
    }
  }, []);
  
  const getCurrentUser = async () => {
    try {
      setIsLoading(true);
      // Use the imported authService singleton
      const response = await authService.getCurrentUser();
      
      if (response.success && response.data) {
        setUser(response.data);
      } else {
        // Token might be invalid or expired
        localStorage.removeItem('auth_token');
        setUser(null);
      }
    } catch (error) {
      console.error("Failed to fetch user data:", error);
      localStorage.removeItem('auth_token');
      setUser(null);
    } finally {
      setIsLoading(false);
    }
  };
  
  const login = async (username: string, password: string) => {
    try {
      // Use the imported authService singleton
      const response = await authService.login(username, password);
      
      if (response.success && response.data) {
        localStorage.setItem('auth_token', response.data.token);
        
        // Get user data
        await getCurrentUser();
        
        return { success: true };
      }
      
      return { 
        success: false, 
        message: response.message || 'Login failed' 
      };
    } catch (error) {
      console.error("Login error:", error);
      return { 
        success: false, 
        message: 'An error occurred during login' 
      };
    }
  };
  
  const logout = async () => {
    try {
      // Use the imported authService singleton
      await authService.logout();
    } catch (error) {
      console.error("Logout error:", error);
    } finally {
      // Remove token and user regardless of API call success
      localStorage.removeItem('auth_token');
      setUser(null);
      router.push('/login');
    }
  };
  
  const register = async (username: string, email: string, password: string) => {
    try {
      // Use the imported authService singleton
      const response = await authService.register({ username, email, password });
      
      if (response.success) {
        // Registration successful, now login
        return login(username, password);
      }
      
      return { 
        success: false, 
        message: response.message || 'Registration failed' 
      };
    } catch (error) {
      console.error("Registration error:", error);
      return { 
        success: false, 
        message: 'An error occurred during registration' 
      };
    }
  };
  
  // Check if user is admin
  const isAdmin = user?.role === 'admin';
  
  const value = {
    user,
    isLoading,
    isAdmin,
    login,
    logout,
    register,
  };
  
  return (
    <AuthContext.Provider value={value}>
      {children}
    </AuthContext.Provider>
  );
};

export const useAuth = () => {
  const context = useContext(AuthContext);
  
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  
  return context;
};
