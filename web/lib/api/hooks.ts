"use client";

import { useEffect, useState } from 'react';
import { ApiResponse } from './types';

/**
 * Custom hook for API data fetching with loading and error states
 * 
 * @param fetchFn - Function that returns a Promise resolving to an ApiResponse
 * @param dependencies - Array of dependencies that trigger refetching when changed
 */
export function useApiQuery<T>(
  fetchFn: () => Promise<ApiResponse<T>>,
  dependencies: any[] = []
) {
  const [data, setData] = useState<T | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);
  const [isRefetching, setIsRefetching] = useState<boolean>(false);

  // Function to fetch data
  const fetchData = async () => {
    try {
      setIsLoading(true);
      setError(null);
      const response = await fetchFn();
      
      if (response.success && response.data) {
        setData(response.data);
      } else {
        setError(response.error || response.message || 'Unknown error');
        setData(null);
      }
    } catch (err: any) {
      setError(err.message || 'An error occurred');
      setData(null);
    } finally {
      setIsLoading(false);
      setIsRefetching(false);
    }
  };

  // Initial data fetch
  useEffect(() => {
    fetchData();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...dependencies]);

  // Function to manually trigger refetch
  const refetch = () => {
    setIsRefetching(true);
    return fetchData();
  };

  return { data, isLoading, isRefetching, error, refetch };
}

/**
 * Custom hook for API mutations with loading and error states
 * 
 * @param mutationFn - Function that takes input data and returns a Promise
 */
export function useApiMutation<T, I = any>(
  mutationFn: (input: I) => Promise<ApiResponse<T>>
) {
  const [data, setData] = useState<T | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);

  // Function to execute the mutation
  const mutate = async (input: I) => {
    try {
      setIsLoading(true);
      setError(null);
      const response = await mutationFn(input);
      
      if (response.success) {
        setData(response.data || null);
        return response;
      } else {
        setError(response.error || response.message || 'Unknown error');
        throw new Error(response.error || response.message || 'Unknown error');
      }
    } catch (err: any) {
      const errorMessage = err.message || 'An error occurred';
      setError(errorMessage);
      throw err;
    } finally {
      setIsLoading(false);
    }
  };

  // Function to reset state
  const reset = () => {
    setData(null);
    setError(null);
    setIsLoading(false);
  };

  return { mutate, data, isLoading, error, reset };
}
