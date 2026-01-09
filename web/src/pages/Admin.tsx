import { useState } from 'react';
import { Shield, Activity, Settings } from 'lucide-react';
import SecurityAuditLog from '../components/Admin/SecurityAuditLog';
import InstanceStats from '../components/Admin/InstanceStats';
import PoWConfig from '../components/Admin/PoWConfig';

type Tab = 'stats' | 'audit' | 'pow';

export default function Admin() {
  const [activeTab, setActiveTab] = useState<Tab>('stats');

  const tabs = [
    { id: 'stats' as Tab, label: 'Statistics', icon: Activity },
    { id: 'audit' as Tab, label: 'Security Audit', icon: Shield },
    { id: 'pow' as Tab, label: 'PoW Config', icon: Settings },
  ];

  return (
    <div className="container mx-auto px-4 py-8">
      {/* Header */}
      <div className="mb-8">
        <h1 className="text-3xl font-bold mb-2 flex items-center gap-2">
          <Shield className="w-8 h-8 text-indigo-500" />
          Admin Panel
        </h1>
        <p className="text-gray-400">
          Manage instance settings, moderation, and security
        </p>
      </div>

      {/* Tabs */}
      <div className="mb-6 border-b border-gray-800">
        <div className="flex gap-1 overflow-x-auto">
          {tabs.map((tab) => {
            const Icon = tab.icon;
            return (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id)}
                className={`flex items-center gap-2 px-4 py-3 border-b-2 transition-colors whitespace-nowrap ${
                  activeTab === tab.id
                    ? 'border-indigo-500 text-indigo-400'
                    : 'border-transparent text-gray-400 hover:text-gray-300'
                }`}
              >
                <Icon className="w-5 h-5" />
                {tab.label}
              </button>
            );
          })}
        </div>
      </div>

      {/* Tab Content */}
      <div>
        {activeTab === 'stats' && <InstanceStats />}
        {activeTab === 'audit' && <SecurityAuditLog />}
        {activeTab === 'pow' && <PoWConfig />}
      </div>
    </div>
  );
}
