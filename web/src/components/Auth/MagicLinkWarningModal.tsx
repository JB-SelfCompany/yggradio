import { AlertTriangle, Copy, Check } from 'lucide-react';
import { useState } from 'react';

interface MagicLinkWarningModalProps {
  magicLink: string;
  onDismiss: () => void;
}

export default function MagicLinkWarningModal({ magicLink, onDismiss }: MagicLinkWarningModalProps) {
  const [copied, setCopied] = useState(false);
  const [copyError, setCopyError] = useState(false);

  // Copy magic link to clipboard
  const handleCopy = async () => {
    setCopyError(false);
    try {
      // Try modern clipboard API first
      if (navigator.clipboard && navigator.clipboard.writeText) {
        await navigator.clipboard.writeText(magicLink);
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
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
          setCopied(true);
          setTimeout(() => setCopied(false), 2000);
        } catch (err) {
          console.error('Fallback copy failed:', err);
          setCopyError(true);
          setTimeout(() => setCopyError(false), 3000);
        }

        document.body.removeChild(textArea);
      }
    } catch (err) {
      console.error('Failed to copy magic link:', err);
      setCopyError(true);
      setTimeout(() => setCopyError(false), 3000);
    }
  };
  return (
    <div className="fixed inset-0 bg-black bg-opacity-70 flex items-center justify-center z-50 p-4">
      <div className="bg-gray-900 rounded-lg max-w-md w-full p-6 space-y-4 border border-yellow-700">
        {/* Header */}
        <div className="flex items-center gap-3 text-yellow-500">
          <AlertTriangle className="w-7 h-7 flex-shrink-0" />
          <h2 className="text-xl font-bold">Save Your Magic Link!</h2>
        </div>

        {/* Warning message */}
        <div className="space-y-3">
          <p className="text-gray-300 text-sm leading-relaxed">
            This is your <strong>only way</strong> to log back in. Make sure to save it somewhere safe:
          </p>

          <ul className="space-y-2 text-sm text-gray-400">
            <li className="flex items-start gap-2">
              <span className="text-green-400 flex-shrink-0">✓</span>
              <span>Bookmark this page in your browser</span>
            </li>
            <li className="flex items-start gap-2">
              <span className="text-green-400 flex-shrink-0">✓</span>
              <span>Save in a password manager (recommended)</span>
            </li>
            <li className="flex items-start gap-2">
              <span className="text-green-400 flex-shrink-0">✓</span>
              <span>Write it down and store securely</span>
            </li>
          </ul>
        </div>

        {/* Magic link display */}
        <div className="bg-gray-800 p-3 rounded border border-gray-700">
          <label className="block text-xs font-medium text-gray-400 mb-2">
            Your Magic Link:
          </label>
          <div className="flex gap-2">
            <input
              type="text"
              value={magicLink}
              readOnly
              className="flex-1 bg-gray-900 text-xs font-mono text-gray-200 p-2 rounded border border-gray-700 break-all"
              onClick={(e) => e.currentTarget.select()}
            />
            <button
              onClick={handleCopy}
              className="px-3 py-2 bg-indigo-600 hover:bg-indigo-700 rounded border border-indigo-500 transition-colors flex items-center justify-center"
              title="Copy to clipboard"
            >
              {copied ? (
                <Check className="w-4 h-4 text-green-400" />
              ) : (
                <Copy className="w-4 h-4" />
              )}
            </button>
          </div>
          {copyError && (
            <p className="text-xs text-red-400 mt-2">
              Failed to copy. Please select and copy the link manually.
            </p>
          )}
        </div>

        {/* Warning box */}
        <div className="p-3 bg-red-900 bg-opacity-30 border border-red-700 rounded">
          <p className="text-red-200 text-xs">
            <strong>⚠️ Warning:</strong> If you lose this link, you'll lose access to your account permanently. There's no password recovery!
          </p>
        </div>

        {/* Confirm button */}
        <button
          onClick={onDismiss}
          className="w-full py-3 px-4 bg-indigo-600 hover:bg-indigo-700 rounded-lg font-semibold transition-colors"
        >
          I've Saved It, Continue
        </button>
      </div>
    </div>
  );
}
