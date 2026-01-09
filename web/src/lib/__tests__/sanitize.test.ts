import { describe, it, expect } from 'vitest';
import {
  sanitizeHTML,
  stripHTML,
  sanitizeText,
  sanitizeURL,
  escapeHTML,
  validateAudioFile,
  validateImageFile,
} from '../sanitize';

describe('Security: sanitizeHTML', () => {
  it('should allow safe HTML tags', () => {
    const input = '<p>Hello <strong>World</strong></p>';
    const result = sanitizeHTML(input);
    expect(result).toContain('<p>');
    expect(result).toContain('<strong>');
    expect(result).toContain('Hello');
  });

  it('should remove script tags (XSS prevention)', () => {
    const input = '<p>Hello</p><script>alert("XSS")</script>';
    const result = sanitizeHTML(input);
    expect(result).not.toContain('<script>');
    expect(result).not.toContain('alert');
    expect(result).toContain('Hello');
  });

  it('should remove onclick handlers (XSS prevention)', () => {
    const input = '<p onclick="alert(\'XSS\')">Click me</p>';
    const result = sanitizeHTML(input);
    expect(result).not.toContain('onclick');
    expect(result).not.toContain('alert');
    expect(result).toContain('Click me');
  });

  it('should remove iframe tags', () => {
    const input = '<iframe src="evil.com"></iframe>';
    const result = sanitizeHTML(input);
    expect(result).not.toContain('<iframe>');
    expect(result).not.toContain('evil.com');
  });

  it('should remove javascript: protocol in links', () => {
    const input = '<a href="javascript:alert(\'XSS\')">Click</a>';
    const result = sanitizeHTML(input);
    expect(result).not.toContain('javascript:');
    expect(result).not.toContain('alert');
  });

  it('should remove data attributes', () => {
    const input = '<p data-evil="script">Hello</p>';
    const result = sanitizeHTML(input);
    expect(result).not.toContain('data-evil');
  });

  it('should handle empty input', () => {
    expect(sanitizeHTML('')).toBe('');
  });

  it('should handle SVG XSS attempts', () => {
    const input = '<svg><script>alert("XSS")</script></svg>';
    const result = sanitizeHTML(input);
    expect(result).not.toContain('<svg>');
    expect(result).not.toContain('<script>');
  });

  it('should remove onerror handlers from images', () => {
    const input = '<img src="x" onerror="alert(\'XSS\')">';
    const result = sanitizeHTML(input);
    expect(result).not.toContain('onerror');
    expect(result).not.toContain('alert');
  });
});

describe('Security: stripHTML', () => {
  it('should remove all HTML tags', () => {
    const input = '<p>Hello <strong>World</strong></p>';
    const result = stripHTML(input);
    expect(result).toBe('Hello World');
    expect(result).not.toContain('<');
    expect(result).not.toContain('>');
  });

  it('should remove malicious HTML', () => {
    const input = '<script>alert("XSS")</script>Safe text';
    const result = stripHTML(input);
    expect(result).not.toContain('<script>');
    expect(result).toContain('Safe text');
  });

  it('should handle empty input', () => {
    expect(stripHTML('')).toBe('');
  });
});

describe('Security: sanitizeText', () => {
  it('should remove null bytes', () => {
    const input = 'Hello\x00World';
    const result = sanitizeText(input);
    expect(result).toBe('HelloWorld');
    expect(result).not.toContain('\x00');
  });

  it('should remove control characters', () => {
    const input = 'Hello\x01\x02\x03World';
    const result = sanitizeText(input);
    expect(result).toBe('HelloWorld');
  });

  it('should preserve newlines and tabs', () => {
    const input = 'Hello\nWorld\tTest';
    const result = sanitizeText(input);
    expect(result).toContain('\n');
    expect(result).toContain('\t');
  });

  it('should truncate text longer than 5000 characters', () => {
    const input = 'A'.repeat(6000);
    const result = sanitizeText(input);
    expect(result.length).toBe(5000);
  });

  it('should handle empty input', () => {
    expect(sanitizeText('')).toBe('');
  });

  it('should handle Unicode characters correctly', () => {
    const input = 'Hello 世界 🌍';
    const result = sanitizeText(input);
    expect(result).toBe(input);
  });
});

describe('Security: sanitizeURL', () => {
  it('should accept valid HTTP URLs', () => {
    const input = 'http://example.com/path';
    const result = sanitizeURL(input);
    expect(result).toBe(input);
  });

  it('should accept valid HTTPS URLs', () => {
    const input = 'https://example.com/path';
    const result = sanitizeURL(input);
    expect(result).toBe(input);
  });

  it('should reject javascript: protocol', () => {
    const input = 'javascript:alert("XSS")';
    const result = sanitizeURL(input);
    expect(result).toBeNull();
  });

  it('should reject data: protocol', () => {
    const input = 'data:text/html,<script>alert("XSS")</script>';
    const result = sanitizeURL(input);
    expect(result).toBeNull();
  });

  it('should reject file: protocol', () => {
    const input = 'file:///etc/passwd';
    const result = sanitizeURL(input);
    expect(result).toBeNull();
  });

  it('should reject URLs longer than 2048 characters', () => {
    const input = 'https://example.com/' + 'a'.repeat(2100);
    const result = sanitizeURL(input);
    expect(result).toBeNull();
  });

  it('should reject invalid URLs', () => {
    const input = 'not a url';
    const result = sanitizeURL(input);
    expect(result).toBeNull();
  });

  it('should handle URLs with query parameters', () => {
    const input = 'https://example.com/path?foo=bar&baz=qux';
    const result = sanitizeURL(input);
    expect(result).toBeTruthy();
  });

  it('should normalize URLs', () => {
    const input = 'https://example.com:443/path';
    const result = sanitizeURL(input);
    expect(result).toBeTruthy();
  });
});

describe('Security: escapeHTML', () => {
  it('should escape ampersands', () => {
    const input = 'Tom & Jerry';
    const result = escapeHTML(input);
    expect(result).toBe('Tom &amp; Jerry');
  });

  it('should escape less than and greater than', () => {
    const input = '<script>alert("XSS")</script>';
    const result = escapeHTML(input);
    expect(result).toContain('&lt;');
    expect(result).toContain('&gt;');
    expect(result).not.toContain('<script>');
  });

  it('should escape quotes', () => {
    const input = 'He said "hello"';
    const result = escapeHTML(input);
    expect(result).toContain('&quot;');
  });

  it('should escape single quotes', () => {
    const input = "It's working";
    const result = escapeHTML(input);
    expect(result).toContain('&#x27;');
  });

  it('should escape forward slashes', () => {
    const input = '</script>';
    const result = escapeHTML(input);
    expect(result).toContain('&#x2F;');
  });

  it('should handle empty input', () => {
    expect(escapeHTML('')).toBe('');
  });

  it('should escape all dangerous characters together', () => {
    const input = '<tag attr="value" & \'test\'>/script';
    const result = escapeHTML(input);
    expect(result).not.toContain('<');
    expect(result).not.toContain('>');
    expect(result).not.toContain('"');
    expect(result).toContain('&amp;');
  });
});

describe('Security: validateAudioFile', () => {
  it('should accept valid MP3 file', () => {
    const file = new File(['audio'], 'test.mp3', { type: 'audio/mpeg' });
    const result = validateAudioFile(file);
    expect(result.valid).toBe(true);
    expect(result.error).toBeUndefined();
  });

  it('should accept valid OGG file', () => {
    const file = new File(['audio'], 'test.ogg', { type: 'audio/ogg' });
    const result = validateAudioFile(file);
    expect(result.valid).toBe(true);
  });

  it('should reject non-audio file', () => {
    const file = new File(['data'], 'test.txt', { type: 'text/plain' });
    const result = validateAudioFile(file);
    expect(result.valid).toBe(false);
    expect(result.error).toContain('Invalid file type');
  });

  it('should reject file larger than 500MB', () => {
    const largeFile = new File(['x'], 'large.mp3', { type: 'audio/mpeg' });
    Object.defineProperty(largeFile, 'size', { value: 600 * 1024 * 1024 });
    const result = validateAudioFile(largeFile);
    expect(result.valid).toBe(false);
    expect(result.error).toContain('too large');
  });

  it('should accept files under 500MB', () => {
    const file = new File(['x'], 'test.mp3', { type: 'audio/mpeg' });
    Object.defineProperty(file, 'size', { value: 100 * 1024 * 1024 });
    const result = validateAudioFile(file);
    expect(result.valid).toBe(true);
  });

  it('should reject executable disguised as audio', () => {
    const file = new File(['exe'], 'virus.exe', { type: 'application/x-msdownload' });
    const result = validateAudioFile(file);
    expect(result.valid).toBe(false);
  });
});

describe('Security: validateImageFile', () => {
  it('should accept valid JPEG file', () => {
    const file = new File(['image'], 'test.jpg', { type: 'image/jpeg' });
    const result = validateImageFile(file);
    expect(result.valid).toBe(true);
  });

  it('should accept valid PNG file', () => {
    const file = new File(['image'], 'test.png', { type: 'image/png' });
    const result = validateImageFile(file);
    expect(result.valid).toBe(true);
  });

  it('should reject non-image file', () => {
    const file = new File(['data'], 'test.txt', { type: 'text/plain' });
    const result = validateImageFile(file);
    expect(result.valid).toBe(false);
    expect(result.error).toContain('Invalid image type');
  });

  it('should reject image larger than 10MB', () => {
    const largeFile = new File(['x'], 'large.jpg', { type: 'image/jpeg' });
    Object.defineProperty(largeFile, 'size', { value: 15 * 1024 * 1024 });
    const result = validateImageFile(largeFile);
    expect(result.valid).toBe(false);
    expect(result.error).toContain('too large');
  });

  it('should accept WebP images', () => {
    const file = new File(['image'], 'test.webp', { type: 'image/webp' });
    const result = validateImageFile(file);
    expect(result.valid).toBe(true);
  });

  it('should reject SVG files (potential XSS vector)', () => {
    const file = new File(['svg'], 'test.svg', { type: 'image/svg+xml' });
    const result = validateImageFile(file);
    expect(result.valid).toBe(false);
  });
});
