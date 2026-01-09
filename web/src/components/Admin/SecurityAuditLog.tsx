import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { Shield, AlertTriangle, Info, AlertCircle } from 'lucide-react';
import { api } from '../../lib/api';

interface AuditEntry {
  id: number;
  eventType: string;
  severity: 'low' | 'medium' | 'high' | 'critical';
  ipv6Address: string | null;
  pubkey: string | null;
  endpoint: string;
  details: string;
  createdAt: string;
}

export default function SecurityAuditLog() {
  const [severityFilter, setSeverityFilter] = useState<string>('all');
  const [page, setPage] = useState(1);
  const limit = 50;

  const { data, isLoading } = useQuery<{ entries: AuditEntry[]; total: number }>({
    queryKey: ['admin-audit-log', severityFilter, page],
    queryFn: () => {
      const params = new URLSearchParams({
        limit: limit.toString(),
        offset: ((page - 1) * limit).toString(),
      });
      if (severityFilter !== 'all') {
        params.append('severity', severityFilter);
      }
      return api.get<{ entries: AuditEntry[]; total: number }>(
        `/admin/audit?${params.toString()}`,
        true
      );
    },
    refetchInterval: 10000, // Refresh every 10 seconds
  });

  const getSeverityIcon = (severity: AuditEntry['severity']) => {
    switch (severity) {
      case 'critical':
        return <AlertCircle className="w-5 h-5 text-red-500" />;
      case 'high':
        return <AlertTriangle className="w-5 h-5 text-orange-500" />;
      case 'medium':
        return <Info className="w-5 h-5 text-yellow-500" />;
      case 'low':
        return <Shield className="w-5 h-5 text-blue-500" />;
    }
  };

  const getSeverityColor = (severity: AuditEntry['severity']) => {
    switch (severity) {
      case 'critical':
        return 'bg-red-900 text-red-200';
      case 'high':
        return 'bg-orange-900 text-orange-200';
      case 'medium':
        return 'bg-yellow-900 text-yellow-200';
      case 'low':
        return 'bg-blue-900 text-blue-200';
    }
  };

  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleString();
  };

  const totalPages = data ? Math.ceil(data.total / limit) : 0;

  return (
    <div className="space-y-6">
      {/* Filters */}
      <div className="flex flex-wrap gap-2">
        {['all', 'critical', 'high', 'medium', 'low'].map((severity) => (
          <button
            key={severity}
            onClick={() => {
              setSeverityFilter(severity);
              setPage(1);
            }}
            className={`px-4 py-2 rounded-lg transition-colors ${
              severityFilter === severity
                ? 'bg-indigo-600 text-white'
                : 'bg-gray-800 text-gray-300 hover:bg-gray-700'
            }`}
          >
            {severity === 'all' ? 'All' : severity.charAt(0).toUpperCase() + severity.slice(1)}
          </button>
        ))}
      </div>

      {/* Log Entries */}
      {isLoading ? (
        <div className="text-center py-12 text-gray-400">Loading audit log...</div>
      ) : data && data.entries.length > 0 ? (
        <>
          <div className="space-y-2">
            {data.entries.map((entry) => (
              <div
                key={entry.id}
                className="bg-gray-900 border border-gray-800 rounded-lg p-4 hover:border-gray-700 transition-colors"
              >
                <div className="flex items-start gap-4">
                  <div className="flex-shrink-0 mt-0.5">
                    {getSeverityIcon(entry.severity)}
                  </div>

                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2 mb-2">
                      <span className={`px-2 py-1 rounded text-xs font-medium ${getSeverityColor(entry.severity)}`}>
                        {entry.severity}
                      </span>
                      <span className="text-sm text-gray-500">{entry.eventType}</span>
                      <span className="text-sm text-gray-600">·</span>
                      <span className="text-sm text-gray-500">{formatDate(entry.createdAt)}</span>
                    </div>

                    <div className="text-gray-300 text-sm mb-2">{entry.details}</div>

                    <div className="flex flex-wrap gap-4 text-xs text-gray-500">
                      {entry.endpoint && (
                        <div>
                          <span className="text-gray-600">Endpoint:</span> {entry.endpoint}
                        </div>
                      )}
                      {entry.ipv6Address && (
                        <div>
                          <span className="text-gray-600">IPv6:</span>{' '}
                          <span className="font-mono">{entry.ipv6Address}</span>
                        </div>
                      )}
                      {entry.pubkey && (
                        <div>
                          <span className="text-gray-600">Pubkey:</span>{' '}
                          <span className="font-mono">
                            {entry.pubkey.substring(0, 8)}...{entry.pubkey.substring(entry.pubkey.length - 8)}
                          </span>
                        </div>
                      )}
                    </div>
                  </div>
                </div>
              </div>
            ))}
          </div>

          {/* Pagination */}
          {totalPages > 1 && (
            <div className="flex items-center justify-between">
              <div className="text-sm text-gray-400">
                Showing {(page - 1) * limit + 1} - {Math.min(page * limit, data.total)} of {data.total}
              </div>
              <div className="flex gap-2">
                <button
                  onClick={() => setPage(page - 1)}
                  disabled={page === 1}
                  className="px-4 py-2 bg-gray-800 hover:bg-gray-700 disabled:opacity-50 disabled:cursor-not-allowed rounded-lg transition-colors"
                >
                  Previous
                </button>
                <button
                  onClick={() => setPage(page + 1)}
                  disabled={page === totalPages}
                  className="px-4 py-2 bg-gray-800 hover:bg-gray-700 disabled:opacity-50 disabled:cursor-not-allowed rounded-lg transition-colors"
                >
                  Next
                </button>
              </div>
            </div>
          )}
        </>
      ) : (
        <div className="bg-gray-900 border border-gray-800 rounded-lg p-8 text-center text-gray-400">
          <Shield className="w-12 h-12 mx-auto mb-4 text-gray-600" />
          <p>No audit entries found</p>
        </div>
      )}
    </div>
  );
}
