"use client";

import { 
  createContext, 
  useContext, 
  useEffect, 
  useState, 
  ReactNode
} from 'react';
import { useRouter } from 'next/navigation';
import { authService, userService } from '@/lib/api';
import { User } from '@/lib/api/types';

interface AuthContextType {
  user: User | null;
  isLoading: boolean;
  isAuthenticated: boolean;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  refreshUser: () => Promise<void>;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [isAuthenticated, setIsAuthenticated] = useState<boolean>(false);
  const router = useRouter();

  // Load user on initial mount
  useEffect(() => {
    const loadUser = async () => {
      try {
        // Check if we have a token
        if (authService.isAuthenticated()) {
          // Attempt to get current user data
          const response = await userService.getCurrentUser();
          if (response.success && response.data) {
            setUser(response.data);
            setIsAuthenticated(true);
          } else {
            // Token might be invalid/expired
            authService.logout();
            setIsAuthenticated(false);
            setUser(null);
          }
        } else {
          setIsAuthenticated(false);
        }
      } catch (error) {
        console.error('Error loading user:', error);
        setIsAuthenticated(false);
      } finally {
        setIsLoading(false);
      }
    };

    loadUser();
  }, []);

  // Login function
  const login = async (username: string, password: string) => {
    setIsLoading(true);
    
    try {
      const response = await authService.login({
        username,
        password
      });

      if (response.success && response.data) {
        // Get user data after successful login
        const userResponse = await userService.getCurrentUser();
        if (userResponse.success && userResponse.data) {
          setUser(userResponse.data);
          setIsAuthenticated(true);
        }
      } else {
        throw new Error('Login failed');
      }
    } catch (error) {
      console.error('Login error:', error);
      throw error;
    } finally {
      setIsLoading(false);
    }
  };

  // Logout function
  const logout = async () => {
    setIsLoading(true);
    
    try {
      await authService.logout();
    } catch (error) {
      console.error('Logout error:', error);
    } finally {
      setUser(null);
      setIsAuthenticated(false);
      setIsLoading(false);
      router.push('/login');
    }
  };

  // Refresh user data
  const refreshUser = async () => {
    if (!isAuthenticated) return;
    
    try {
      const response = await userService.getCurrentUser();
      if (response.success && response.data) {
        setUser(response.data);
      }
    } catch (error) {
      console.error('Error refreshing user:', error);
    }
  };

  return (
    <AuthContext.Provider
      value={{
        user,
        isLoading,
        isAuthenticated,
        login,
        logout,
        refreshUser
      }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export const useAuth = () => {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
};

// Protected route wrapper
export function withAuth<P extends object>(
  Component: React.ComponentType<P>
): React.FC<P> {
  return function ProtectedRoute(props: P) {
    const { isAuthenticated, isLoading } = useAuth();
    const router = useRouter();

    useEffect(() => {
      if (!isLoading && !isAuthenticated) {
        router.replace('/login');
      }
    }, [isAuthenticated, isLoading, router]);

    if (isLoading) {
      return <div>Loading...</div>;
    }

    return isAuthenticated ? <Component {...props} /> : null;
  };
}
