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

import React, { useContext, useEffect, useMemo, useState } from 'react';
import { Button } from '@douyinfe/semi-ui';
import { IconClose } from '@douyinfe/semi-icons';
import { Megaphone } from 'lucide-react';
import { marked } from 'marked';
import { useTranslation } from 'react-i18next';
import { StatusContext } from '../../context/Status';

const DISMISS_KEY_PREFIX = 'promo_popup_dismissed';

function hashString(input) {
  let hash = 0;
  if (!input) return '0';

  for (let i = 0; i < input.length; i += 1) {
    hash = (hash << 5) - hash + input.charCodeAt(i);
    hash |= 0;
  }

  return Math.abs(hash).toString(36);
}

function isDismissed(key) {
  try {
    return localStorage.getItem(key) === 'true';
  } catch {
    return false;
  }
}

const PromoPopup = () => {
  const { t } = useTranslation();
  const [statusState] = useContext(StatusContext);
  const content = String(statusState?.status?.promo_popup_content || '').trim();
  const contentHash = useMemo(() => hashString(content), [content]);
  const dismissKey = `${DISMISS_KEY_PREFIX}:${contentHash}`;
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    if (!content) {
      setVisible(false);
      return;
    }
    setVisible(!isDismissed(dismissKey));
  }, [content, dismissKey]);

  const htmlContent = useMemo(() => {
    if (!content) return '';
    return marked.parse(content);
  }, [content]);

  const handleClose = () => {
    try {
      localStorage.setItem(dismissKey, 'true');
    } catch {
      // ignore localStorage failures
    }
    setVisible(false);
  };

  if (!content || !visible) return null;

  return (
    <aside
      className='fixed right-4 bottom-4 w-[min(24rem,calc(100vw-2rem))] rounded-2xl border border-semi-color-border bg-semi-color-bg-2 shadow-2xl overflow-hidden'
      style={{ zIndex: 1000 }}
      aria-label={t('通知')}
    >
      <div className='h-1 bg-semi-color-primary' />
      <div className='flex gap-3 p-4'>
        <div className='mt-1 flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-semi-color-primary-light-default text-semi-color-primary'>
          <Megaphone size={18} />
        </div>
        <div className='min-w-0 flex-1'>
          <div className='mb-2 flex items-center justify-between gap-3'>
            <div className='text-sm font-semibold text-semi-color-text-0'>
              {t('通知')}
            </div>
            <Button
              type='tertiary'
              theme='borderless'
              size='small'
              icon={<IconClose />}
              onClick={handleClose}
              aria-label={t('关闭通知')}
            />
          </div>
          <div
            className='max-h-[min(52vh,22rem)] overflow-y-auto pr-1 text-sm text-semi-color-text-1 card-content-scroll'
            dangerouslySetInnerHTML={{ __html: htmlContent }}
          />
        </div>
      </div>
    </aside>
  );
};

export default PromoPopup;
