import { useState } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { User, LogOut, Bell, Menu, Loader2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { useAuth } from '@/lib/auth/auth-context';

interface HeaderProps {
  onMenuToggle: () => void;
}

export function Header({ onMenuToggle }: HeaderProps) {
  const router = useRouter();
  const { user, logout } = useAuth();
  const [isLoggingOut, setIsLoggingOut] = useState(false);

  const handleLogout = async () => {
    if (isLoggingOut) return;
    
    setIsLoggingOut(true);
    try {
      await logout();
      // No need to redirect as the auth context will handle it
    } catch (error) {
      console.error('Logout failed:', error);
      setIsLoggingOut(false);
    }
  };

  return (
    <header className="sticky top-0 z-30 flex h-16 items-center border-b bg-background px-4">
      <Button
        variant="ghost"
        size="icon"
        className="md:hidden mr-2"
        onClick={onMenuToggle}
      >
        <Menu className="h-5 w-5" />
        <span className="sr-only">Toggle menu</span>
      </Button>
      
      <div className="flex-1">
        <Link href="/dashboard" className="flex items-center space-x-2">
          <span className="font-bold text-xl">GKiPass</span>
        </Link>
      </div>
      
      <div className="flex items-center gap-2">
        <Button variant="ghost" size="icon" aria-label="Notifications">
          <Bell className="h-5 w-5" />
        </Button>
        
        <div className="border-l h-6 mx-2" />
        
        <div className="flex items-center">
          <Button
            variant="ghost"
            size="sm"
            className="gap-2"
            asChild
          >
            <Link href="/profile">
              <User className="h-4 w-4" />
              <span className="hidden sm:inline-block">{user?.username || 'Profile'}</span>
            </Link>
          </Button>
          
          <Button
            variant="ghost"
            size="sm"
            className="gap-2"
            onClick={handleLogout}
            disabled={isLoggingOut}
          >
            <LogOut className="h-4 w-4" />
            <span className="hidden sm:inline-block">Logout</span>
          </Button>
        </div>
      </div>
    </header>
  );
}
