import { Link } from 'react-router-dom';
import { Radio, Shield } from 'lucide-react';
import { useEffect } from 'react';
import { useAuthStore } from '../stores/authStore';

export default function Home() {
  const loginWithMagicLink = useAuthStore((state) => state.loginWithMagicLink);

  useEffect(() => {
    // Check for ?first_login=true query parameter
    const params = new URLSearchParams(window.location.search);
    if (params.get('first_login') === 'true') {
      // Remove query parameters from URL immediately (clean up)
      window.history.replaceState({}, '', window.location.pathname);

      // Verify magic link session (no modal needed - user already has the link)
      loginWithMagicLink()
        .catch((err) => {
          console.error('Failed to verify magic link session:', err);
          // Show error to user
          alert('Authentication failed. Please try using your magic link again.');
        });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []); // Run only once on mount

  return (
    <>
    <div className="container mx-auto px-4 py-12">
      {/* Hero Section */}
      <div className="text-center mb-16">
        <h1 className="text-5xl font-bold mb-4">
          Welcome to <span className="text-indigo-500">YggRadio</span>
        </h1>
        <p className="text-xl text-gray-400 max-w-2xl mx-auto">
          Decentralized P2P radio platform built on Yggdrasil mesh network.
          Stream live radio stations without central servers.
        </p>
      </div>

      {/* Features Grid */}
      <div className="grid md:grid-cols-2 gap-8 mb-16">
        <Link
          to="/stations"
          className="bg-gray-900 border border-gray-800 rounded-lg p-8 hover:border-indigo-500 transition-colors group"
        >
          <Radio className="w-12 h-12 text-indigo-500 mb-4 group-hover:scale-110 transition-transform" />
          <h3 className="text-2xl font-bold mb-2">Live Radio</h3>
          <p className="text-gray-400">
            Browse and listen to live radio stations streaming in the Yggdrasil network.
          </p>
        </Link>

        <div className="bg-gray-900 border border-gray-800 rounded-lg p-8">
          <Shield className="w-12 h-12 text-indigo-500 mb-4" />
          <h3 className="text-2xl font-bold mb-2">Secure & Private</h3>
          <p className="text-gray-400">
            End-to-end encrypted with TLS 1.3, Ed25519 authentication,
            and federated moderation. Your content, your control.
          </p>
        </div>
      </div>

      {/* Key Features */}
      <div className="bg-gray-900 border border-gray-800 rounded-lg p-8">
        <h2 className="text-2xl font-bold mb-6">Key Features</h2>
        <div className="grid md:grid-cols-2 gap-6">
          <div>
            <h4 className="font-semibold mb-2 text-indigo-400">Decentralized Architecture</h4>
            <p className="text-gray-400 text-sm">
              Built on Yggdrasil mesh network with P2P relay for popular content.
              No central servers, no single point of failure.
            </p>
          </div>
          <div>
            <h4 className="font-semibold mb-2 text-indigo-400">Icecast Compatible</h4>
            <p className="text-gray-400 text-sm">
              Full compatibility with Icecast protocol. Stream with OBS, BUTT, or ffmpeg.
              Listen with any media player.
            </p>
          </div>
          <div>
            <h4 className="font-semibold mb-2 text-indigo-400">Federated Moderation</h4>
            <p className="text-gray-400 text-sm">
              Three-tier moderation system with Proof-of-Work spam protection.
            </p>
          </div>
          <div>
            <h4 className="font-semibold mb-2 text-indigo-400">Self-Hosting Ready</h4>
            <p className="text-gray-400 text-sm">
              Single binary with embedded database and web interface.
              Run your own instance with minimal setup.
            </p>
          </div>
        </div>
      </div>
    </div>
    </>
  );
}
