"use client";

import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { useRouter } from 'next/navigation';
import Image from 'next/image';
import { authService } from '@/lib/api';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from '@/components/ui/card';

// Form validation schema
const registerSchema = z.object({
  username: z.string().min(3, 'Username must be at least 3 characters'),
  email: z.string().email('Invalid email address'),
  password: z.string().min(6, 'Password must be at least 6 characters'),
  confirmPassword: z.string(),
  captchaId: z.string().optional(),
  captchaCode: z.string().optional(),
}).refine((data) => data.password === data.confirmPassword, {
  message: "Passwords don't match",
  path: ["confirmPassword"],
});

type RegisterFormValues = z.infer<typeof registerSchema>;

export function RegisterForm() {
  const router = useRouter();
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [captcha, setCaptcha] = useState<{ captchaId: string; imageData: string } | null>(null);

  // Initialize form
  const {
    register,
    handleSubmit,
    formState: { errors },
    setValue,
  } = useForm<RegisterFormValues>({
    resolver: zodResolver(registerSchema),
    defaultValues: {
      username: '',
      email: '',
      password: '',
      confirmPassword: '',
    },
  });

  // Load captcha if required
  const loadCaptcha = async () => {
    try {
      const response = await authService.getCaptcha();
      if (response.data) {
        setCaptcha(response.data);
        setValue('captchaId', response.data.captchaId);
      }
    } catch (error) {
      console.error('Failed to load captcha:', error);
    }
  };

  // Handle form submission
  const onSubmit = async (data: RegisterFormValues) => {
    setIsLoading(true);
    setError(null);

    try {
      const { confirmPassword, ...registerData } = data;
      const response = await authService.register(registerData);
      
      if (response.data) {
        // Registration successful, redirect to login page
        router.push('/login?registered=true');
      }
    } catch (error: any) {
      setError(error.message || 'An error occurred during registration');
      // Refresh captcha on error if it was being used
      if (captcha) {
        loadCaptcha();
      }
    } finally {
      setIsLoading(false);
    }
  };

  // Load captcha on component mount if system requires it
  /*
  useEffect(() => {
    loadCaptcha();
  }, []);
  */

  return (
    <Card className="w-full max-w-md mx-auto">
      <CardHeader>
        <CardTitle className="text-center">Create an Account</CardTitle>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
          {error && (
            <div className="p-3 text-sm bg-red-50 text-red-600 rounded-md">{error}</div>
          )}
          
          <div className="space-y-2">
            <label htmlFor="username" className="text-sm font-medium">
              Username
            </label>
            <Input
              id="username"
              placeholder="Username"
              {...register('username')}
              disabled={isLoading}
            />
            {errors.username && (
              <p className="text-sm text-red-500">{errors.username.message}</p>
            )}
          </div>
          
          <div className="space-y-2">
            <label htmlFor="email" className="text-sm font-medium">
              Email
            </label>
            <Input
              id="email"
              type="email"
              placeholder="your.email@example.com"
              {...register('email')}
              disabled={isLoading}
            />
            {errors.email && (
              <p className="text-sm text-red-500">{errors.email.message}</p>
            )}
          </div>
          
          <div className="space-y-2">
            <label htmlFor="password" className="text-sm font-medium">
              Password
            </label>
            <Input
              id="password"
              type="password"
              placeholder="••••••••"
              {...register('password')}
              disabled={isLoading}
            />
            {errors.password && (
              <p className="text-sm text-red-500">{errors.password.message}</p>
            )}
          </div>
          
          <div className="space-y-2">
            <label htmlFor="confirmPassword" className="text-sm font-medium">
              Confirm Password
            </label>
            <Input
              id="confirmPassword"
              type="password"
              placeholder="••••••••"
              {...register('confirmPassword')}
              disabled={isLoading}
            />
            {errors.confirmPassword && (
              <p className="text-sm text-red-500">{errors.confirmPassword.message}</p>
            )}
          </div>

          {captcha && (
            <div className="space-y-2">
              <label htmlFor="captcha" className="text-sm font-medium">
                Verification Code
              </label>
              <div className="flex items-center gap-2">
                <Input
                  id="captcha"
                  placeholder="Enter code"
                  {...register('captchaCode')}
                  disabled={isLoading}
                  className="flex-1"
                />
                <div className="h-10 border rounded overflow-hidden">
                  <Image 
                    src={`data:image/png;base64,${captcha.imageData}`} 
                    alt="Captcha" 
                    width={120} 
                    height={40}
                    onClick={() => loadCaptcha()}
                    className="cursor-pointer"
                  />
                </div>
              </div>
            </div>
          )}
          
          <Button 
            type="submit" 
            className="w-full" 
            disabled={isLoading}
          >
            {isLoading ? 'Creating Account...' : 'Register'}
          </Button>
        </form>
      </CardContent>
      <CardFooter className="flex justify-center">
        <p className="text-sm text-gray-500">
          Already have an account?{' '}
          <a href="/login" className="text-primary hover:underline">
            Login
          </a>
        </p>
      </CardFooter>
    </Card>
  );
}
