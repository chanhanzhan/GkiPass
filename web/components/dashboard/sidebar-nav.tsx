"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { cn } from "@/lib/utils";
import { useAuth } from "@/lib/auth/auth-context";

// Icons imports from lucide-react
import { 
  Home, 
  Server, 
  Layers,
  Workflow, 
  LineChart, 
  Package, 
  Wallet, 
  Settings, 
  Users, 
  LifeBuoy
} from "lucide-react";

interface NavItem {
  title: string;
  href: string;
  icon: React.ReactNode;
  adminOnly?: boolean;
}

export function SidebarNav() {
  const pathname = usePathname();
  const { isAdmin } = useAuth();
  
  const navItems: NavItem[] = [
    {
      title: "Dashboard",
      href: "/dashboard/dashboard",
      icon: <Home className="h-5 w-5" />,
    },
    {
      title: "Nodes",
      href: "/dashboard/nodes",
      icon: <Server className="h-5 w-5" />,
    },
    {
      title: "Node Groups",
      href: "/dashboard/nodegroups",
      icon: <Layers className="h-5 w-5" />,
    },
    {
      title: "Tunnels",
      href: "/dashboard/tunnels",
      icon: <Workflow className="h-5 w-5" />,
    },
    {
      title: "Monitoring",
      href: "/dashboard/monitoring",
      icon: <LineChart className="h-5 w-5" />,
    },
    {
      title: "Plans",
      href: "/plans",
      icon: <Package className="h-5 w-5" />,
    },
    {
      title: "Wallet",
      href: "/wallet",
      icon: <Wallet className="h-5 w-5" />,
    },
    {
      title: "Users",
      href: "/dashboard/users",
      icon: <Users className="h-5 w-5" />,
      adminOnly: true,
    },
    {
      title: "Settings",
      href: "/settings",
      icon: <Settings className="h-5 w-5" />,
    },
    {
      title: "Help",
      href: "/help",
      icon: <LifeBuoy className="h-5 w-5" />,
    },
  ];

  // Using the auth context to check if user is admin

  return (
    <nav className="flex flex-col space-y-1">
      {navItems.map((item) => {
        // Skip admin-only items if user is not admin
        if (item.adminOnly && !isAdmin) {
          return null;
        }

        const isActive = pathname === item.href || pathname.startsWith(`${item.href}/`);

        return (
          <Link
            key={item.href}
            href={item.href}
            className={cn(
              "flex items-center px-3 py-2 text-sm font-medium rounded-md",
              isActive
                ? "bg-primary text-primary-foreground"
                : "text-muted-foreground hover:bg-muted hover:text-foreground"
            )}
          >
            {item.icon}
            <span className="ml-3">{item.title}</span>
          </Link>
        );
      })}
    </nav>
  );
}
