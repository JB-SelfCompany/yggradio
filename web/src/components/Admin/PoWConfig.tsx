import { useState, useEffect } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Save } from 'lucide-react';
import { api, fetchCSRFToken } from '../../lib/api';

interface PoWConfig {
  commentDifficulty: number;
  accountDifficulty: number;
  stationDifficulty: number;
}

export default function PoWConfig() {
  const queryClient = useQueryClient();
  const [config, setConfig] = useState<PoWConfig>({
    commentDifficulty: 4,
    accountDifficulty: 8,
    stationDifficulty: 12,
  });

  // Fetch current config
  const { isLoading, data } = useQuery<PoWConfig>({
    queryKey: ['admin-pow-config'],
    queryFn: () => api.get<PoWConfig>('/admin/pow-config', true),
  });

  useEffect(() => {
    if (data) {
      setConfig(data);
    }
  }, [data]);

  // Update config mutation
  const updateMutation = useMutation({
    mutationFn: async (newConfig: PoWConfig) => {
      const csrfToken = await fetchCSRFToken();
      return api.put('/admin/pow-config', newConfig, true, csrfToken);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-pow-config'] });
    },
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    updateMutation.mutate(config);
  };

  const estimateTime = (difficulty: number): string => {
    const attempts = Math.pow(2, difficulty);
    const hashesPerSecond = 1000000; // Approximate
    const seconds = attempts / hashesPerSecond;

    if (seconds < 0.001) {
      return `~${(seconds * 1000000).toFixed(0)}μs`;
    } else if (seconds < 1) {
      return `~${(seconds * 1000).toFixed(1)}ms`;
    } else {
      return `~${seconds.toFixed(2)}s`;
    }
  };

  const difficulties = [
    {
      key: 'commentDifficulty' as keyof PoWConfig,
      label: 'Comment Creation',
      description: 'Difficulty for posting comments',
      min: 4,
      max: 16,
    },
    {
      key: 'accountDifficulty' as keyof PoWConfig,
      label: 'Account Creation',
      description: 'Difficulty for creating user accounts',
      min: 6,
      max: 16,
    },
    {
      key: 'stationDifficulty' as keyof PoWConfig,
      label: 'Station Creation',
      description: 'Difficulty for creating radio stations',
      min: 8,
      max: 20,
    },
  ];

  if (isLoading) {
    return (
      <div className="text-center py-12 text-gray-400">
        Loading configuration...
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="bg-blue-900 bg-opacity-30 border border-blue-700 rounded-lg p-4 text-blue-200 text-sm">
        <strong>Note:</strong> Proof-of-Work difficulty prevents spam by requiring computational work.
        Higher values provide better protection but may frustrate legitimate users.
      </div>

      <form onSubmit={handleSubmit} className="space-y-6">
        {difficulties.map((item) => (
          <div key={item.key} className="bg-gray-900 border border-gray-800 rounded-lg p-6">
            <div className="flex items-start justify-between mb-4">
              <div>
                <h3 className="font-semibold mb-1">{item.label}</h3>
                <p className="text-sm text-gray-400">{item.description}</p>
              </div>
              <span className="text-2xl font-bold text-indigo-400">
                {config[item.key]} bits
              </span>
            </div>

            <div className="space-y-2">
              <input
                type="range"
                min={item.min}
                max={item.max}
                value={config[item.key]}
                onChange={(e) =>
                  setConfig({ ...config, [item.key]: parseInt(e.target.value) })
                }
                className="w-full h-2 bg-gray-700 rounded-lg appearance-none cursor-pointer accent-indigo-600"
              />
              <div className="flex justify-between text-sm text-gray-500">
                <span>{item.min} bits (easier)</span>
                <span>{item.max} bits (harder)</span>
              </div>
              <div className="text-center text-sm text-gray-400">
                Estimated solving time: {estimateTime(config[item.key])}
              </div>
            </div>
          </div>
        ))}

        <div className="flex justify-end">
          <button
            type="submit"
            disabled={updateMutation.isPending}
            className="flex items-center gap-2 px-6 py-3 bg-indigo-600 hover:bg-indigo-700 disabled:bg-gray-700 rounded-lg font-semibold transition-colors"
          >
            <Save className="w-5 h-5" />
            {updateMutation.isPending ? 'Saving...' : 'Save Configuration'}
          </button>
        </div>

        {updateMutation.isSuccess && (
          <div className="bg-green-900 bg-opacity-50 border border-green-700 rounded-lg p-4 text-green-200 text-sm">
            Configuration saved successfully
          </div>
        )}

        {updateMutation.isError && (
          <div className="bg-red-900 bg-opacity-50 border border-red-700 rounded-lg p-4 text-red-200 text-sm">
            Failed to save configuration. Please try again.
          </div>
        )}
      </form>
    </div>
  );
}
