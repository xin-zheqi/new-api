/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import DOMPurify from 'dompurify';
import React, { useEffect, useMemo, useRef } from 'react';

const iframeSandbox =
  'allow-forms allow-popups allow-popups-to-escape-sandbox allow-presentation';

const inlineSanitizeOptions = {
  ADD_ATTR: ['target'],
  FORBID_ATTR: ['srcdoc'],
  FORBID_TAGS: [
    'base',
    'embed',
    'iframe',
    'link',
    'meta',
    'object',
    'script',
    'style',
  ],
};

const isolatedSanitizeOptions = {
  ADD_ATTR: [
    'allowfullscreen',
    'autoplay',
    'class',
    'controls',
    'default',
    'id',
    'kind',
    'label',
    'loading',
    'loop',
    'muted',
    'playsinline',
    'poster',
    'preload',
    'referrerpolicy',
    'rel',
    'srclang',
    'style',
    'target',
  ],
  ADD_TAGS: ['audio', 'iframe', 'picture', 'source', 'style', 'track', 'video'],
  FORBID_ATTR: ['srcdoc'],
  FORBID_TAGS: ['base', 'embed', 'link', 'meta', 'object', 'script'],
  FORCE_BODY: true,
};

const isolatedBaseStyles = `
<style>
  :host { display: block; width: 100%; color: inherit; font: inherit; }
  *, *::before, *::after { box-sizing: border-box; }
  img, video, iframe { max-width: 100%; }
  iframe { border: 0; }
</style>
`;

function hardenHtml(html, allowIframes) {
  if (typeof document === 'undefined') return html;

  const template = document.createElement('template');
  template.innerHTML = html;

  template.content.querySelectorAll('a[target="_blank"]').forEach((link) => {
    const rel = new Set(
      (link.getAttribute('rel') || '').split(/\s+/).filter(Boolean),
    );
    rel.add('noopener');
    rel.add('noreferrer');
    link.setAttribute('rel', [...rel].join(' '));
  });

  if (allowIframes) {
    template.content.querySelectorAll('iframe').forEach((frame) => {
      frame.removeAttribute('srcdoc');
      frame.setAttribute('sandbox', iframeSandbox);
      frame.setAttribute('referrerpolicy', 'no-referrer');
      if (!frame.hasAttribute('loading')) frame.setAttribute('loading', 'lazy');
    });
  }

  return template.innerHTML;
}

function sanitizeHtml(html, isolated) {
  const options = isolated ? isolatedSanitizeOptions : inlineSanitizeOptions;
  return hardenHtml(DOMPurify.sanitize(String(html || ''), options), isolated);
}

function IsolatedHtml({ html, className, style }) {
  const containerRef = useRef(null);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const shadowRoot =
      container.shadowRoot || container.attachShadow({ mode: 'open' });
    const applicationStyles = [
      ...document.head.querySelectorAll('style, link[rel="stylesheet"]'),
    ].map((node) => node.cloneNode(true));
    const baseStyles = document.createElement('template');
    baseStyles.innerHTML = isolatedBaseStyles;
    const content = document.createElement('div');
    content.innerHTML = html;

    shadowRoot.replaceChildren(
      ...applicationStyles,
      baseStyles.content,
      content,
    );
  }, [html]);

  return <div ref={containerRef} className={className} style={style} />;
}

const SafeHtml = ({ html, className, style, variant = 'inline' }) => {
  const isolated = variant === 'isolated';
  const sanitizedHtml = useMemo(
    () => sanitizeHtml(html, isolated),
    [html, isolated],
  );

  if (isolated) {
    return (
      <IsolatedHtml html={sanitizedHtml} className={className} style={style} />
    );
  }

  return (
    <div
      className={className}
      style={style}
      dangerouslySetInnerHTML={{ __html: sanitizedHtml }}
    />
  );
};

export default SafeHtml;
