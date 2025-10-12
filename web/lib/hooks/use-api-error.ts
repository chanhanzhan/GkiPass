"use client";

import { useState, useCallback } from 'react';
import { ApiError } from '../api/error-handler';
import { toast } from '../ui/use-toast';

/**
 * Custom hook for handling API errors in React components
 * 
 * @returns Object with error state and handler functions
 */
export function useApiError() {
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(false);

  /**
   * Handles an API error and updates the error state
   * 
   * @param error The error object
   * @param showToast Whether to show a toast notification (default: true)
   */
  const handleError = useCallback((error: any, showToast = true) => {
    let errorMessage: string;

    if (error instanceof ApiError) {
      errorMessage = error.message;
    } else {
      errorMessage = error?.message || 'An unexpected error occurred';
    }

    setError(errorMessage);
    
    if (showToast) {
      toast({
        title: 'Error',
        description: errorMessage,
        variant: 'destructive',
      });
    }
    
    console.error('API Error:', error);
  }, []);

  /**
   * Wraps an async function with loading state and error handling
   * 
   * @param asyncFn The async function to wrap
   * @param onSuccess Optional callback to run on success
   * @returns A function that executes the wrapped async function
   */
  const withErrorHandling = useCallback(<T extends any[], R>(
    asyncFn: (...args: T) => Promise<R>,
    onSuccess?: (result: R) => void
  ) => {
    return async (...args: T): Promise<R | undefined> => {
      setIsLoading(true);
      setError(null);
      
      try {
        const result = await asyncFn(...args);
        
        if (onSuccess) {
          onSuccess(result);
        }
        
        return result;
      } catch (err) {
        handleError(err);
        return undefined;
      } finally {
        setIsLoading(false);
      }
    };
  }, [handleError]);

  /**
   * Clears the current error
   */
  const clearError = useCallback(() => {
    setError(null);
  }, []);

  return {
    error,
    isLoading,
    handleError,
    withErrorHandling,
    clearError,
  };
}

/**
 * Executes an async function with loading state and error handling
 * This is a standalone utility function for use outside of React components
 * 
 * @param asyncFn The async function to execute
 * @param setIsLoading Function to update loading state
 * @param setError Function to update error state
 * @param onSuccess Optional callback to run on success
 * @returns The result of the async function or undefined if an error occurred
 */
export async function executeWithErrorHandling<T>(
  asyncFn: () => Promise<T>,
  setIsLoading?: (loading: boolean) => void,
  setError?: (error: string | null) => void,
  onSuccess?: (result: T) => void
): Promise<T | undefined> {
  if (setIsLoading) setIsLoading(true);
  if (setError) setError(null);
  
  try {
    const result = await asyncFn();
    
    if (onSuccess) {
      onSuccess(result);
    }
    
    return result;
  } catch (error) {
    const errorMessage = error instanceof ApiError
      ? error.message
      : (error as Error)?.message || 'An unexpected error occurred';
    
    if (setError) setError(errorMessage);
    
    console.error('API Error:', error);
    
    // Show toast notification if available
    if (typeof window !== 'undefined') {
      toast({
        title: 'Error',
        description: errorMessage,
        variant: 'destructive',
      });
    }
    
    return undefined;
  } finally {
    if (setIsLoading) setIsLoading(false);
  }
}
