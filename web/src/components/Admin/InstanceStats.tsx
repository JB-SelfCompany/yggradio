import { useQuery } from '@tanstack/react-query';
import { Radio, Users, Database } from 'lucide-react';
import { api } from '../../lib/api';

interface Stats {
  stations: {
    total: number;
    online: number;
    offline: number;
  };
  users: {
    total: number;
    active: number;
  };
  database: {
    size: string;
    tables: number;
  };
}

export default function InstanceStats() {
  const { data: stats, isLoading } = useQuery<Stats>({
    queryKey: ['admin-stats'],
    queryFn: () => api.get<Stats>('/admin/stats', true),
    refetchInterval: 30000, // Refresh every 30 seconds
  });

  if (isLoading || !stats) {
    return (
      <div className="text-center py-12 text-gray-400">
        Loading statistics...
      </div>
    );
  }

  const statCards = [
    {
      icon: Radio,
      label: 'Radio Stations',
      value: stats.stations.total,
      subtitle: `${stats.stations.online} online, ${stats.stations.offline} offline`,
      color: 'text-blue-500',
    },
    {
      icon: Users,
      label: 'Users',
      value: stats.users.total,
      subtitle: `${stats.users.active} active`,
      color: 'text-green-500',
    },
    {
      icon: Database,
      label: 'Database',
      value: stats.database.size,
      subtitle: `${stats.database.tables} tables`,
      color: 'text-yellow-500',
    },
  ];

  return (
    <div className="space-y-6">
      {/* Overview Cards */}
      <div className="grid sm:grid-cols-2 lg:grid-cols-4 gap-6">
        {statCards.map((card) => {
          const Icon = card.icon;
          return (
            <div
              key={card.label}
              className="bg-gray-900 border border-gray-800 rounded-lg p-6"
            >
              <div className="flex items-center justify-between mb-4">
                <h3 className="text-gray-400 text-sm">{card.label}</h3>
                <Icon className={`w-6 h-6 ${card.color}`} />
              </div>
              <div className="text-3xl font-bold mb-1">{card.value}</div>
              <div className="text-sm text-gray-500">{card.subtitle}</div>
            </div>
          );
        })}
      </div>

      {/* Additional Info */}
      <div className="bg-gray-900 border border-gray-800 rounded-lg p-6">
        <h3 className="text-lg font-semibold mb-4">Instance Information</h3>
        <div className="space-y-2 text-sm">
          <div className="flex justify-between">
            <span className="text-gray-400">Version:</span>
            <span>0.1.0</span>
          </div>
          <div className="flex justify-between">
            <span className="text-gray-400">Uptime:</span>
            <span>N/A</span>
          </div>
          <div className="flex justify-between">
            <span className="text-gray-400">Federation:</span>
            <span className="text-green-500">Enabled</span>
          </div>
        </div>
      </div>
    </div>
  );
}
