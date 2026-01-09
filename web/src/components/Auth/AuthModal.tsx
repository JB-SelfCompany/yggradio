import { useState } from 'react';
import { X, Key, Upload, Download, Link2, Copy } from 'lucide-react';
import { useAuthStore } from '../../stores/authStore';
import { usePlayerStore } from '../../stores/playerStore';
import { calculatePoWForChallenge } from '../../lib/pow';

interface AuthModalProps {
  onClose: () => void;
}

export default function AuthModal({ onClose }: AuthModalProps) {
  const [mode, setMode] = useState<'generate' | 'import' | 'magic'>('generate');
  const [publicKeyInput, setPublicKeyInput] = useState('');
  const [privateKeyInput, setPrivateKeyInput] = useState('');
  const [error, setError] = useState('');

  // Magic link state
  const [magicLink, setMagicLink] = useState<string | null>(null);
  const [isGeneratingMagicLink, setIsGeneratingMagicLink] = useState(false);

  const { generateKeyPair, importKeyPair, publicKey, privateKey } = useAuthStore();
  const { currentStreamUrl } = usePlayerStore();

  const handleGenerate = () => {
    generateKeyPair();
    setError('');
  };

  const handleImport = () => {
    try {
      if (!publicKeyInput.trim() || !privateKeyInput.trim()) {
        setError('Both public and private keys are required');
        return;
      }

      importKeyPair(publicKeyInput.trim(), privateKeyInput.trim());
      setError('');
      onClose();
    } catch (err) {
      setError('Invalid key format. Please check your keys.');
    }
  };

  const handleExport = () => {
    if (!publicKey || !privateKey) return;

    const keyData = {
      publicKey,
      privateKey,
      timestamp: new Date().toISOString(),
    };

    const blob = new Blob([JSON.stringify(keyData, null, 2)], {
      type: 'application/json',
    });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `yggradio-keys-${Date.now()}.json`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  };

  const handleImportFile = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    const reader = new FileReader();
    reader.onload = (event) => {
      try {
        const data = JSON.parse(event.target?.result as string);
        if (data.publicKey && data.privateKey) {
          setPublicKeyInput(data.publicKey);
          setPrivateKeyInput(data.privateKey);
          setError('');
        } else {
          setError('Invalid key file format');
        }
      } catch (err) {
        setError('Failed to parse key file');
      }
    };
    reader.readAsText(file);
  };

  // Handle magic link generation
  const handleGenerateMagicLink = async () => {
    setIsGeneratingMagicLink(true);
    setError('');

    try {
      // Calculate PoW for magic link registration
      // Challenge format: register_magic_link_<timestamp>
      // Use current Unix timestamp (in seconds)
      const timestamp = Math.floor(Date.now() / 1000);
      const challenge = `register_magic_link_${timestamp}`;

      // Calculate PoW hash and nonce (16-bit difficulty, ~2-4 seconds on average)
      const {hash, nonce} = await calculatePoWForChallenge(challenge, 16);

      // Send registration request
      const response = await fetch('/api/auth/register/magic-link', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          pow_hash: hash,
          pow_nonce: String(nonce),
          pow_timestamp: timestamp,
        }),
      });

      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}));
        throw new Error(errorData.message || 'Failed to generate magic link');
      }

      const data = await response.json();
      setMagicLink(data.magic_link);
    } catch (err: any) {
      setError(err.message || 'Failed to generate magic link');
    } finally {
      setIsGeneratingMagicLink(false);
    }
  };

  // Copy magic link to clipboard
  const handleCopyLink = async () => {
    if (!magicLink) return;

    try {
      // Try modern clipboard API first
      if (navigator.clipboard && navigator.clipboard.writeText) {
        await navigator.clipboard.writeText(magicLink);
        alert('Magic link copied to clipboard!');
      } else {
        // Fallback for older browsers or non-HTTPS contexts
        const textArea = document.createElement('textarea');
        textArea.value = magicLink;
        textArea.style.position = 'fixed';
        textArea.style.left = '-999999px';
        textArea.style.top = '-999999px';
        document.body.appendChild(textArea);
        textArea.focus();
        textArea.select();

        try {
          document.execCommand('copy');
          alert('Magic link copied to clipboard!');
        } catch (err) {
          console.error('Fallback copy failed:', err);
          alert('Failed to copy. Please copy manually.');
        }

        document.body.removeChild(textArea);
      }
    } catch (err) {
      console.error('Failed to copy magic link:', err);
      alert('Failed to copy. Please copy manually.');
    }
  };

  // Download magic link as text file
  const handleDownloadLink = () => {
    if (!magicLink) return;

    const blob = new Blob([magicLink], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `yggradio-magic-link-${Date.now()}.txt`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  };

  // Check if player is visible (has active stream)
  const isPlayerVisible = !!currentStreamUrl;

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-start sm:items-center justify-center z-50 p-2 sm:p-4 overflow-y-auto">
      <div
        className={`bg-gray-900 rounded-lg max-w-2xl w-full my-2 sm:my-4 overflow-y-auto ${
          isPlayerVisible
            ? 'max-h-[calc(100vh-140px)] sm:max-h-[90vh]'
            : 'max-h-[95vh]'
        }`}
        style={isPlayerVisible ? { marginBottom: '100px' } : undefined}
      >
        {/* Header */}
        <div className="flex items-center justify-between p-6 border-b border-gray-800">
          <div className="flex items-center gap-2">
            <Key className="w-6 h-6 text-indigo-500" />
            <h2 className="text-xl font-bold">Authentication</h2>
          </div>
          <button
            onClick={onClose}
            className="p-2 hover:bg-gray-800 rounded-lg transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Content */}
        <div className="p-6">
          {/* Mode Selector */}
          <div className="grid grid-cols-3 gap-2 mb-6">
            <button
              onClick={() => setMode('generate')}
              className={`py-2 px-3 rounded-lg transition-colors text-sm ${
                mode === 'generate'
                  ? 'bg-indigo-600 text-white'
                  : 'bg-gray-800 text-gray-300 hover:bg-gray-700'
              }`}
            >
              Generate Keys
            </button>
            <button
              onClick={() => setMode('import')}
              className={`py-2 px-3 rounded-lg transition-colors text-sm ${
                mode === 'import'
                  ? 'bg-indigo-600 text-white'
                  : 'bg-gray-800 text-gray-300 hover:bg-gray-700'
              }`}
            >
              Import Keys
            </button>
            <button
              onClick={() => setMode('magic')}
              className={`py-2 px-3 rounded-lg transition-colors text-sm flex items-center justify-center gap-1 ${
                mode === 'magic'
                  ? 'bg-indigo-600 text-white'
                  : 'bg-gray-800 text-gray-300 hover:bg-gray-700'
              }`}
            >
              <Link2 className="w-4 h-4" />
              Magic Link
            </button>
          </div>

          {error && (
            <div className="mb-4 p-3 bg-red-900 bg-opacity-50 border border-red-700 rounded-lg text-red-200 text-sm">
              {error}
            </div>
          )}

          {mode === 'generate' ? (
            <div className="space-y-4">
              <p className="text-gray-400 text-sm">
                Generate a new Ed25519 key pair for authentication. Keep your private key safe!
              </p>

              <button
                onClick={handleGenerate}
                className="w-full py-3 px-4 bg-indigo-600 hover:bg-indigo-700 rounded-lg font-semibold transition-colors"
              >
                Generate Key Pair
              </button>

              {/* Manual key generation instructions */}
              <div className="mt-6 p-4 bg-gray-800 rounded-lg border border-gray-700">
                <h3 className="text-sm font-semibold mb-3 text-gray-200">
                  Advanced: Generate keys manually (more secure)
                </h3>
                <p className="text-xs text-gray-400 mb-3">
                  For advanced users - generate Ed25519 keys using command line tools:
                </p>

                <div className="space-y-3">
                  {/* Python method */}
                  <div>
                    <p className="text-xs font-medium text-gray-300 mb-1">Python (PyNaCl):</p>
                    <div className="bg-gray-900 p-2 rounded overflow-x-auto">
                      <code className="text-xs text-green-400 whitespace-pre">
{`python3 -c "import nacl.signing, base64;
key = nacl.signing.SigningKey.generate();
print('Private:', base64.b64encode(bytes(key)).decode());
print('Public:', base64.b64encode(bytes(key.verify_key)).decode())"`}
                      </code>
                    </div>
                  </div>

                  {/* Node.js method */}
                  <div>
                    <p className="text-xs font-medium text-gray-300 mb-1">Node.js (tweetnacl):</p>
                    <div className="bg-gray-900 p-2 rounded overflow-x-auto">
                      <code className="text-xs text-green-400 whitespace-pre">
{`node -e "const nacl = require('tweetnacl');
const key = nacl.sign.keyPair();
console.log('Private:', Buffer.from(key.secretKey).toString('base64'));
console.log('Public:', Buffer.from(key.publicKey).toString('base64'));"`}
                      </code>
                    </div>
                  </div>

                  {/* OpenSSL method */}
                  <div>
                    <p className="text-xs font-medium text-gray-300 mb-1">OpenSSL (requires manual conversion):</p>
                    <div className="bg-gray-900 p-2 rounded overflow-x-auto">
                      <code className="text-xs text-green-400 whitespace-pre">
{`openssl genpkey -algorithm ed25519 -out key.pem
openssl pkey -in key.pem -pubout -out pubkey.pem`}
                      </code>
                    </div>
                    <p className="text-xs text-gray-500 mt-1">
                      Note: You'll need to convert PEM to base64 manually
                    </p>
                  </div>
                </div>

                <p className="text-xs text-gray-400 mt-3">
                  After generating, use "Import Keys" tab to import them.
                </p>
              </div>

              {publicKey && privateKey && (
                <div className="space-y-4 p-4 bg-gray-800 rounded-lg">
                  <div className="p-3 bg-yellow-900 bg-opacity-30 border border-yellow-700 rounded text-yellow-200 text-sm">
                    Warning: Save these keys! They won't be shown again.
                  </div>

                  <div>
                    <label className="block text-sm font-medium mb-2">
                      Public Key
                    </label>
                    <input
                      type="text"
                      value={publicKey}
                      readOnly
                      className="w-full px-3 py-2 bg-gray-900 border border-gray-700 rounded text-sm font-mono"
                    />
                  </div>

                  <div>
                    <label className="block text-sm font-medium mb-2">
                      Private Key
                    </label>
                    <input
                      type="password"
                      value={privateKey}
                      readOnly
                      className="w-full px-3 py-2 bg-gray-900 border border-gray-700 rounded text-sm font-mono"
                    />
                  </div>

                  <button
                    onClick={handleExport}
                    className="w-full py-2 px-4 bg-gray-700 hover:bg-gray-600 rounded-lg flex items-center justify-center gap-2 transition-colors"
                  >
                    <Download className="w-5 h-5" />
                    Export Keys to File
                  </button>

                  <button
                    onClick={onClose}
                    className="w-full py-2 px-4 bg-indigo-600 hover:bg-indigo-700 rounded-lg transition-colors"
                  >
                    Continue
                  </button>
                </div>
              )}
            </div>
          ) : mode === 'import' ? (
            <div className="space-y-4">
              <p className="text-gray-400 text-sm">
                Import your Ed25519 key pair from a JSON file. This is the most secure way to import your keys.
              </p>

              <div className="p-4 bg-gray-800 rounded-lg border border-gray-700">
                <h3 className="text-sm font-semibold mb-2 text-gray-200">
                  Security Note
                </h3>
                <p className="text-xs text-gray-400">
                  For security, manual key entry is not supported. Please use the JSON file exported from the "Generate Keys" tab or created with command-line tools.
                </p>
              </div>

              {publicKeyInput && privateKeyInput && (
                <div className="p-4 bg-green-900 bg-opacity-30 border border-green-700 rounded-lg">
                  <p className="text-sm text-green-200 mb-2">
                    ✓ Keys loaded successfully
                  </p>
                  <p className="text-xs text-gray-400 font-mono break-all">
                    Public Key: {publicKeyInput.substring(0, 20)}...
                  </p>
                </div>
              )}

              <label className="block">
                <input
                  type="file"
                  accept=".json"
                  onChange={handleImportFile}
                  className="hidden"
                />
                <div className="w-full py-3 px-4 bg-gray-800 hover:bg-gray-700 rounded-lg flex items-center justify-center gap-2 cursor-pointer transition-colors border-2 border-dashed border-gray-600 hover:border-indigo-500">
                  <Upload className="w-5 h-5" />
                  Select Key File (.json)
                </div>
              </label>

              <button
                onClick={handleImport}
                disabled={!publicKeyInput || !privateKeyInput}
                className="w-full py-3 px-4 bg-indigo-600 hover:bg-indigo-700 disabled:bg-gray-700 disabled:text-gray-500 rounded-lg font-semibold transition-colors"
              >
                Import and Login
              </button>
            </div>
          ) : mode === 'magic' ? (
            <div className="space-y-4">
              <div className="space-y-3">
                <p className="text-gray-400 text-sm">
                  Create a magic link for easy login without managing keys.
                </p>

                <div className="p-3 bg-blue-900 bg-opacity-30 border border-blue-700 rounded">
                  <p className="text-blue-200 text-xs">
                    <strong>💡 Note:</strong> You can also use Ed25519 keys for authentication (more secure). Switch to "Generate Keys" tab if you prefer key-based authentication.
                  </p>
                </div>
              </div>

              {!magicLink ? (
                <>
                  <div className="p-4 bg-red-900 bg-opacity-30 border border-red-700 rounded">
                    <p className="text-red-200 text-sm font-semibold mb-2">
                      ⚠️ Before you proceed:
                    </p>
                    <ul className="space-y-2 text-red-200 text-xs">
                      <li className="flex items-start gap-2">
                        <span className="flex-shrink-0">•</span>
                        <span>The magic link will be the only way to access this specific account (you cannot use keys with it)</span>
                      </li>
                      <li className="flex items-start gap-2">
                        <span className="flex-shrink-0">•</span>
                        <span>If you lose the link, you'll lose access permanently - no recovery possible</span>
                      </li>
                      <li className="flex items-start gap-2">
                        <span className="flex-shrink-0">•</span>
                        <span>Anyone with the link can access your account - keep it secret</span>
                      </li>
                      <li className="flex items-start gap-2">
                        <span className="flex-shrink-0">•</span>
                        <span>Must be saved immediately in a password manager or secure location</span>
                      </li>
                      <li className="flex items-start gap-2">
                        <span className="flex-shrink-0">•</span>
                        <span><strong>Alternative:</strong> If you prefer more security, use "Generate Keys" instead</span>
                      </li>
                    </ul>
                  </div>

                  <button
                    onClick={handleGenerateMagicLink}
                    disabled={isGeneratingMagicLink}
                    className="w-full py-3 px-4 bg-indigo-600 hover:bg-indigo-700 disabled:bg-gray-700 rounded-lg font-semibold transition-colors flex items-center justify-center gap-2"
                  >
                    {isGeneratingMagicLink ? (
                      <>
                        <div className="w-5 h-5 border-2 border-white border-t-transparent rounded-full animate-spin" />
                        Generating...
                      </>
                    ) : (
                      <>
                        <Link2 className="w-5 h-5" />
                        I Understand, Generate Magic Link
                      </>
                    )}
                  </button>

                  {isGeneratingMagicLink && (
                    <div className="text-sm text-gray-400 text-center">
                      Calculating Proof of Work... (~2-4 seconds)
                    </div>
                  )}
                </>
              ) : (
                <div className="space-y-4 p-4 bg-gray-800 rounded-lg">
                  <div className="p-3 bg-yellow-900 bg-opacity-30 border border-yellow-700 rounded text-yellow-200 text-sm">
                    <strong>⚠️ CRITICAL:</strong> Save this link NOW! Without it, you cannot access your account.
                  </div>

                  <div className="p-3 bg-blue-900 bg-opacity-30 border border-blue-700 rounded">
                    <p className="text-blue-200 text-xs mb-2 font-semibold">
                      Recommended ways to save your magic link:
                    </p>
                    <ul className="space-y-1 text-blue-200 text-xs">
                      <li className="flex items-start gap-2">
                        <span className="text-green-400">✓</span>
                        <span>Password manager (1Password, Bitwarden, KeePass)</span>
                      </li>
                      <li className="flex items-start gap-2">
                        <span className="text-green-400">✓</span>
                        <span>Browser bookmark (right-click link → Add Bookmark)</span>
                      </li>
                      <li className="flex items-start gap-2">
                        <span className="text-green-400">✓</span>
                        <span>Download as file and store securely</span>
                      </li>
                      <li className="flex items-start gap-2">
                        <span className="text-red-400">✗</span>
                        <span>DO NOT share with anyone</span>
                      </li>
                    </ul>
                  </div>

                  <div>
                    <label className="block text-sm font-medium mb-2">
                      Magic Link
                    </label>
                    <input
                      type="text"
                      value={magicLink}
                      readOnly
                      className="w-full px-3 py-2 bg-gray-900 border border-gray-700 rounded text-sm font-mono break-all"
                    />
                  </div>

                  <div className="grid grid-cols-2 gap-2">
                    <button
                      onClick={handleCopyLink}
                      className="py-2 px-4 bg-gray-700 hover:bg-gray-600 rounded-lg flex items-center justify-center gap-2 transition-colors"
                    >
                      <Copy className="w-4 h-4" />
                      Copy
                    </button>
                    <button
                      onClick={handleDownloadLink}
                      className="py-2 px-4 bg-gray-700 hover:bg-gray-600 rounded-lg flex items-center justify-center gap-2 transition-colors"
                    >
                      <Download className="w-4 h-4" />
                      Download
                    </button>
                  </div>

                  <button
                    onClick={onClose}
                    className="w-full py-2 px-4 bg-indigo-600 hover:bg-indigo-700 rounded-lg transition-colors"
                  >
                    Done
                  </button>
                </div>
              )}
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
}
