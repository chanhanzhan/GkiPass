import { Metadata } from 'next';
import { LoginForm } from '@/components/auth/login-form';

export const metadata: Metadata = {
  title: 'Login - GKiPass',
  description: 'Log in to GKiPass tunnel control plane',
};

export default function LoginPage() {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center py-12 bg-gray-50">
      <div className="w-full max-w-md">
        <div className="mb-8 flex flex-col items-center space-y-2 text-center">
          <h1 className="text-2xl font-bold tracking-tight">
            GKiPass
          </h1>
          <p className="text-sm text-gray-500 dark:text-gray-400">
            Bidirectional Tunnel Control Plane
          </p>
        </div>
        <LoginForm />
      </div>
    </div>
  );
}
