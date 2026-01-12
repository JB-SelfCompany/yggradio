import { Link, useLocation } from 'react-router-dom';
import { Radio, Shield, User, LogOut, Key, Github } from 'lucide-react';
import { useAuthStore } from '../../stores/authStore';
import { useState, useEffect } from 'react';
import AudioPlayer from '../Player/AudioPlayer';
import AuthModal from '../Auth/AuthModal';
import { getServerInfo } from '../../lib/api';

export default function Layout({ children }: { children: React.ReactNode }) {
  const location = useLocation();
  const { isAuthenticated, isAdmin, logout, publicKey } = useAuthStore();
  const [showAuthModal, setShowAuthModal] = useState(false);
  const [version, setVersion] = useState<string>('');

  useEffect(() => {
    // Load server info to get version
    getServerInfo()
      .then((info) => setVersion(info.version))
      .catch((error) => console.error('Failed to load server info:', error));
  }, []);

  const navigation = [
    { name: 'Home', href: '/', icon: Radio },
    { name: 'Stations', href: '/stations', icon: Radio },
  ];

  if (isAdmin) {
    navigation.push({ name: 'Admin', href: '/admin', icon: Shield });
  }

  const isActive = (path: string) => {
    if (path === '/') {
      return location.pathname === '/';
    }
    return location.pathname.startsWith(path);
  };

  const truncatePublicKey = (key: string) => {
    return `${key.substring(0, 6)}...${key.substring(key.length - 6)}`;
  };

  return (
    <div className="min-h-screen bg-gray-950 text-white flex flex-col">
      {/* Header */}
      <header className="bg-gray-900 border-b border-gray-800 sticky top-0 z-40">
        <div className="container mx-auto px-4">
          <div className="flex items-center justify-between h-16">
            {/* Logo */}
            <Link to="/" className="flex items-center gap-2">
              <Radio className="w-8 h-8 text-indigo-500" />
              <span className="text-xl font-bold">YggRadio</span>
            </Link>

            {/* Navigation */}
            <nav className="hidden md:flex items-center gap-1">
              {navigation.map((item) => {
                const Icon = item.icon;
                return (
                  <Link
                    key={item.href}
                    to={item.href}
                    className={`flex items-center gap-2 px-4 py-2 rounded-lg transition-colors ${
                      isActive(item.href)
                        ? 'bg-indigo-600 text-white'
                        : 'text-gray-300 hover:bg-gray-800'
                    }`}
                  >
                    <Icon className="w-5 h-5" />
                    {item.name}
                  </Link>
                );
              })}
            </nav>

            {/* Auth Section */}
            <div className="flex items-center gap-2">
              {isAuthenticated ? (
                <>
                  <div className="hidden sm:flex items-center gap-2 px-3 py-1.5 bg-gray-800 rounded-lg text-sm">
                    <Key className="w-4 h-4 text-indigo-400" />
                    <span className="text-gray-300">
                      {truncatePublicKey(publicKey || '')}
                    </span>
                  </div>
                  <button
                    onClick={logout}
                    className="flex items-center gap-2 px-4 py-2 text-gray-300 hover:bg-gray-800 rounded-lg transition-colors"
                  >
                    <LogOut className="w-5 h-5" />
                    <span className="hidden sm:inline">Logout</span>
                  </button>
                </>
              ) : (
                <button
                  onClick={() => setShowAuthModal(true)}
                  className="flex items-center gap-2 px-4 py-2 bg-indigo-600 hover:bg-indigo-700 rounded-lg transition-colors"
                >
                  <User className="w-5 h-5" />
                  <span className="hidden sm:inline">Login</span>
                </button>
              )}
            </div>
          </div>

          {/* Mobile Navigation */}
          <nav className="md:hidden flex items-center gap-1 pb-2 overflow-x-auto">
            {navigation.map((item) => {
              const Icon = item.icon;
              return (
                <Link
                  key={item.href}
                  to={item.href}
                  className={`flex items-center gap-2 px-3 py-1.5 rounded-lg whitespace-nowrap transition-colors ${
                    isActive(item.href)
                      ? 'bg-indigo-600 text-white'
                      : 'text-gray-300 hover:bg-gray-800'
                  }`}
                >
                  <Icon className="w-4 h-4" />
                  <span className="text-sm">{item.name}</span>
                </Link>
              );
            })}
          </nav>
        </div>
      </header>

      {/* Main Content */}
      <main className="flex-1 pb-24">
        {children}
      </main>

      {/* Audio Player */}
      <AudioPlayer />

      {/* Footer */}
      <footer className="bg-gray-900 border-t border-gray-800 py-4">
        <div className="container mx-auto px-4">
          <div className="flex items-center justify-center sm:justify-between gap-4 text-sm text-gray-400">
            <div className="flex items-center gap-2 order-2 sm:order-1">
              <span>© 2026 JB-SelfCompany</span>
              {version && (
                <>
                  <span className="text-gray-600">•</span>
                  <span className="text-gray-500">v{version}</span>
                </>
              )}
            </div>
            <a
              href="https://github.com/JB-SelfCompany/yggradio"
              target="_blank"
              rel="noopener noreferrer"
              className="flex items-center gap-2 hover:text-indigo-400 transition-colors order-1 sm:order-2 absolute sm:relative right-4 sm:right-auto"
            >
              <Github className="w-4 h-4" />
              <span>GitHub</span>
            </a>
          </div>
        </div>
      </footer>

      {/* Auth Modal */}
      {showAuthModal && (
        <AuthModal onClose={() => setShowAuthModal(false)} />
      )}
    </div>
  );
}
