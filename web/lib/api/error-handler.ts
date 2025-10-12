/**
 * API Error Handler
 * 
 * This utility provides standardized error handling for API requests.
 * It helps with:
 * - Parsing error responses from the API
 * - Handling network errors
 * - Providing user-friendly error messages
 * - Logging errors for debugging
 */

import { ApiResponse } from './types';

export class ApiError extends Error {
  public status?: number;
  public code?: string;
  public originalError?: any;

  constructor(message: string, status?: number, code?: string, originalError?: any) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.originalError = originalError;
  }

  static fromError(error: any): ApiError {
    // Handle fetch network errors
    if (error.name === 'TypeError' && error.message === 'Failed to fetch') {
      return new ApiError(
        'Unable to connect to the server. Please check your internet connection.',
        0,
        'NETWORK_ERROR',
        error
      );
    }

    // Handle timeout errors
    if (error.name === 'AbortError') {
      return new ApiError(
        'The request timed out. Please try again later.',
        0,
        'TIMEOUT_ERROR',
        error
      );
    }

    // If it's already an ApiError, return it
    if (error instanceof ApiError) {
      return error;
    }

    // Default error
    return new ApiError(
      error.message || 'An unexpected error occurred',
      error.status || 500,
      error.code || 'UNKNOWN_ERROR',
      error
    );
  }

  // Get a user-friendly message based on status code
  static getUserFriendlyMessage(status?: number): string {
    switch (status) {
      case 400:
        return 'The request was invalid. Please check your input.';
      case 401:
        return 'You need to be logged in to access this resource.';
      case 403:
        return 'You do not have permission to access this resource.';
      case 404:
        return 'The requested resource was not found.';
      case 409:
        return 'There was a conflict with the current state of the resource.';
      case 422:
        return 'The request was well-formed but contains semantic errors.';
      case 429:
        return 'Too many requests. Please try again later.';
      case 500:
        return 'An internal server error occurred. Please try again later.';
      case 502:
      case 503:
      case 504:
        return 'The server is currently unavailable. Please try again later.';
      default:
        return 'An unexpected error occurred. Please try again later.';
    }
  }
}

/**
 * Handles API errors in a standardized way
 * 
 * @param error The error object
 * @param defaultMessage Default message to show if error can't be parsed
 * @returns Standardized ApiError object
 */
export function handleApiError(error: any, defaultMessage = 'An error occurred'): ApiError {
  console.error('API Error:', error);

  // If it's already an ApiError, return it
  if (error instanceof ApiError) {
    return error;
  }

  // Try to parse error from API response
  if (error.response) {
    try {
      const errorData = error.response.data || {};
      const status = error.response.status;
      const message = errorData.message || ApiError.getUserFriendlyMessage(status);
      const code = errorData.code || `ERROR_${status}`;
      
      return new ApiError(message, status, code, error);
    } catch (parseError) {
      console.error('Error parsing API error response:', parseError);
    }
  }

  // If we couldn't parse the error, create a generic one
  return ApiError.fromError(error);
}

/**
 * Wraps an API call with standardized error handling
 * 
 * @param apiCall The API call function to wrap
 * @returns The API response or throws a standardized ApiError
 */
export async function withErrorHandling<T>(
  apiCall: () => Promise<ApiResponse<T>>
): Promise<ApiResponse<T>> {
  try {
    return await apiCall();
  } catch (error) {
    throw handleApiError(error);
  }
}
