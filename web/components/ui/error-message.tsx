"use client";

import { AlertCircle, XCircle } from 'lucide-react';
import { ReactNode } from 'react';

interface ErrorMessageProps {
  message: string | null;
  className?: string;
  onDismiss?: () => void;
  variant?: 'error' | 'warning';
}

export function ErrorMessage({ 
  message, 
  className = '',
  onDismiss,
  variant = 'error'
}: ErrorMessageProps) {
  if (!message) return null;
  
  const isError = variant === 'error';
  const bgColor = isError ? 'bg-red-50' : 'bg-yellow-50';
  const textColor = isError ? 'text-red-600' : 'text-yellow-800';
  const borderColor = isError ? 'border-red-200' : 'border-yellow-200';
  const Icon = isError ? XCircle : AlertCircle;
  
  return (
    <div className={`p-3 rounded-md border ${bgColor} ${textColor} ${borderColor} flex items-start ${className}`}>
      <Icon className="h-5 w-5 mr-2 flex-shrink-0 mt-0.5" />
      <div className="flex-grow">{message}</div>
      {onDismiss && (
        <button 
          onClick={onDismiss}
          className={`${textColor} hover:opacity-80 ml-2 flex-shrink-0`}
          aria-label="Dismiss"
        >
          <XCircle className="h-5 w-5" />
        </button>
      )}
    </div>
  );
}

interface ApiErrorHandlerProps {
  error: string | null;
  children: ReactNode;
  onRetry?: () => void;
}

export function ApiErrorHandler({ error, children, onRetry }: ApiErrorHandlerProps) {
  if (error) {
    return (
      <div className="space-y-4">
        <ErrorMessage message={error} />
        {onRetry && (
          <div className="flex justify-center">
            <button
              className="px-4 py-2 bg-blue-500 text-white rounded-md hover:bg-blue-600"
              onClick={onRetry}
            >
              Try Again
            </button>
          </div>
        )}
      </div>
    );
  }
  
  return <>{children}</>;
}
