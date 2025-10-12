'use client';

import { useState, useEffect } from 'react';
import { usePathname } from 'next/navigation';
import { Header } from '@/components/dashboard/header';
import { SidebarNav } from '@/components/dashboard/sidebar-nav';
import { ProtectedRoute } from '@/lib/auth/protected-route';

export default function DashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const [isSidebarOpen, setIsSidebarOpen] = useState(true);
  const pathname = usePathname();

  // Close sidebar on mobile when navigating
  useEffect(() => {
    const isSmallScreen = window.innerWidth < 768;
    if (isSmallScreen) {
      setIsSidebarOpen(false);
    }
  }, [pathname]);

  return (
    <ProtectedRoute>
      <div className="flex min-h-screen flex-col">
        <Header onMenuToggle={() => setIsSidebarOpen(!isSidebarOpen)} />
        
        <div className="flex flex-1">
          {/* Sidebar */}
          <aside 
            className={`fixed inset-y-0 left-0 z-20 w-64 transform bg-background border-r pt-16 transition-transform duration-200 md:translate-x-0 md:static md:z-0 ${
              isSidebarOpen ? 'translate-x-0' : '-translate-x-full'
            }`}
          >
            <div className="h-full overflow-y-auto py-4 px-3">
              <SidebarNav />
            </div>
          </aside>
          
          {/* Backdrop for mobile */}
          {isSidebarOpen && (
            <div 
              className="fixed inset-0 z-10 bg-black/50 md:hidden"
              onClick={() => setIsSidebarOpen(false)}
            />
          )}
          
          {/* Main content */}
          <main className="flex-1 overflow-x-hidden p-4 md:p-6">{children}</main>
        </div>
      </div>
    </ProtectedRoute>
  );
}
