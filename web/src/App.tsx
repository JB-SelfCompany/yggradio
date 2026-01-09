import { BrowserRouter as Router, Routes, Route, Navigate } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useEffect } from 'react';
import Layout from './components/Layout/Layout';
import Home from './pages/Home';
import StationBrowser from './pages/StationBrowser';
import Admin from './pages/Admin';
import { useAuthStore } from './stores/authStore';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 1,
      staleTime: 5 * 60 * 1000, // 5 minutes
    },
  },
});

function App() {
  const { isAuthenticated, authMethod, loginWithMagicLink } = useAuthStore();

  // Check for magic link session on app startup
  useEffect(() => {
    // If not authenticated OR authenticated but missing authMethod (corrupt state)
    // Try to validate magic link session from cookie
    if (!isAuthenticated || !authMethod) {
      console.log('App: Checking for magic link session...');
      loginWithMagicLink()
        .then(() => {
          console.log('App: Magic link session restored');
        })
        .catch(() => {
          // Silently fail - user is not logged in via magic link
          console.log('App: No magic link session found');
        });
    }
  }, []); // Run once on mount

  return (
    <QueryClientProvider client={queryClient}>
      <Router>
        <Layout>
          <Routes>
            <Route path="/" element={<Home />} />
            <Route path="/stations" element={<StationBrowser />} />
            <Route path="/admin" element={<ProtectedRoute><Admin /></ProtectedRoute>} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </Layout>
      </Router>
    </QueryClientProvider>
  );
}

// Protected route wrapper for admin pages
function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, isAdmin } = useAuthStore();

  if (!isAuthenticated || !isAdmin) {
    return <Navigate to="/" replace />;
  }

  return <>{children}</>;
}

export default App;
