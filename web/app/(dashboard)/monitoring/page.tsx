'use client';

import MonitoringDashboard from '@/components/monitoring/monitoring-dashboard';

export default function MonitoringPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Monitoring Dashboard</h1>
        <p className="text-muted-foreground">
          Real-time monitoring and analytics for your network infrastructure.
        </p>
      </div>
      
      <MonitoringDashboard />
    </div>
  );
}