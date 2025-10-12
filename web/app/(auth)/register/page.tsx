import { Metadata } from 'next';
import { RegisterForm } from '@/components/auth/register-form';

export const metadata: Metadata = {
  title: 'Register - GKiPass',
  description: 'Create a new account on GKiPass tunnel control plane',
};

export default function RegisterPage() {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center py-12 bg-gray-50">
      <div className="w-full max-w-md">
        <div className="mb-8 flex flex-col items-center space-y-2 text-center">
          <h1 className="text-2xl font-bold tracking-tight">
            GKiPass
          </h1>
          <p className="text-sm text-gray-500 dark:text-gray-400">
            Create an account to get started
          </p>
        </div>
        <RegisterForm />
      </div>
    </div>
  );
}
