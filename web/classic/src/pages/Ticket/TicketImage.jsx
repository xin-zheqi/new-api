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

import React, { useEffect, useRef, useState } from 'react';
import {
  Button,
  ImagePreview,
  Skeleton,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import { Eye, ImagePlus, Trash2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API, showError } from '../../helpers';
import {
  TICKET_ALLOWED_IMAGE_TYPES,
  TICKET_IMAGE_ACCEPT,
  TICKET_MAX_IMAGE_SIZE,
  validateTicketImage,
} from './ticket';

export function TicketImagePicker({ file, onChange, disabled = false }) {
  const { t } = useTranslation();
  const inputRef = useRef(null);
  const [previewVisible, setPreviewVisible] = useState(false);
  const [previewUrl, setPreviewUrl] = useState('');

  useEffect(() => {
    if (!file) {
      setPreviewUrl('');
      return undefined;
    }
    const url = URL.createObjectURL(file);
    setPreviewUrl(url);
    return () => URL.revokeObjectURL(url);
  }, [file]);

  const selectFile = (event) => {
    const selectedFile = event.target.files?.[0];
    event.target.value = '';
    if (!selectedFile) return;
    const validationError = validateTicketImage(selectedFile, t);
    if (validationError) {
      showError(validationError);
      return;
    }
    onChange(selectedFile);
  };

  return (
    <div className='flex flex-wrap items-center gap-2'>
      <input
        ref={inputRef}
        type='file'
        accept={TICKET_IMAGE_ACCEPT}
        className='hidden'
        disabled={disabled}
        aria-label={t('添加图片')}
        onChange={selectFile}
      />
      <Button
        size='small'
        type='tertiary'
        theme='outline'
        icon={<ImagePlus size={15} />}
        disabled={disabled}
        onClick={() => inputRef.current?.click()}
      >
        {file ? t('更换图片') : t('添加图片')}
      </Button>
      {file && (
        <>
          <button
            type='button'
            className='max-w-[260px] truncate text-left text-sm text-[var(--semi-color-primary)] hover:underline disabled:cursor-not-allowed'
            disabled={disabled}
            onClick={() => setPreviewVisible(true)}
          >
            {file.name} ({(file.size / 1024 / 1024).toFixed(2)} MiB)
          </button>
          <Tooltip content={t('移除图片')}>
            <Button
              aria-label={t('移除图片')}
              icon={<Trash2 size={15} />}
              icononly
              size='small'
              type='danger'
              theme='borderless'
              disabled={disabled}
              onClick={() => onChange(null)}
            />
          </Tooltip>
        </>
      )}
      <Typography.Text type='tertiary' size='small'>
        {t('每条消息最多一张 JPG、PNG 或 WebP 图片，不能超过 5 MiB。')}
      </Typography.Text>
      {previewUrl && (
        <ImagePreview
          src={previewUrl}
          visible={previewVisible}
          onVisibleChange={setPreviewVisible}
        />
      )}
    </div>
  );
}

export function TicketAttachment({ ticketId, attachment }) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const [imageUrl, setImageUrl] = useState('');
  const [previewVisible, setPreviewVisible] = useState(false);
  const [requestedKey, setRequestedKey] = useState('');
  const attachmentKey = `${ticketId || ''}:${attachment?.id || ''}`;

  useEffect(() => {
    setRequestedKey('');
    setImageUrl('');
    setPreviewVisible(false);
    setError(false);
    setLoading(false);
  }, [attachmentKey]);

  useEffect(() => {
    const attachmentId = attachment?.id;
    if (requestedKey !== attachmentKey) {
      return undefined;
    }
    if (!ticketId || !attachmentId) return undefined;

    const controller = new AbortController();
    let active = true;
    let objectUrl = '';
    setLoading(true);
    setError(false);
    setImageUrl('');

    API.get(`/api/ticket/${ticketId}/attachment/${attachmentId}`, {
      responseType: 'blob',
      signal: controller.signal,
      disableDuplicate: true,
      skipErrorHandler: true,
    })
      .then((response) => {
        const blob = response.data;
        const mimeType = blob?.type?.split(';', 1)[0]?.trim()?.toLowerCase();
        if (
          !(blob instanceof Blob) ||
          blob.size <= 0 ||
          blob.size > TICKET_MAX_IMAGE_SIZE ||
          !TICKET_ALLOWED_IMAGE_TYPES.has(mimeType)
        ) {
          throw new Error('Unsupported attachment type');
        }
        if (!active) return;
        objectUrl = URL.createObjectURL(blob);
        setImageUrl(objectUrl);
        setPreviewVisible(true);
      })
      .catch((requestError) => {
        if (active && requestError?.name !== 'CanceledError') {
          setError(true);
          setLoading(false);
          setRequestedKey('');
        }
      })
      .finally(() => {
        if (active) setLoading(false);
      });

    return () => {
      active = false;
      controller.abort();
      if (objectUrl) URL.revokeObjectURL(objectUrl);
    };
  }, [attachmentKey, requestedKey]);

  if (loading) {
    return (
      <Skeleton.Title
        style={{ width: 160, height: 36, borderRadius: 6 }}
        active
      />
    );
  }
  if (error) {
    return (
      <div className='flex flex-wrap items-center gap-2'>
        <Typography.Text type='danger' size='small'>
          {t('图片加载失败')}
        </Typography.Text>
        <Button
          size='small'
          type='tertiary'
          onClick={() => {
            setError(false);
            setRequestedKey(attachmentKey);
          }}
        >
          {t('重试')}
        </Button>
      </div>
    );
  }

  if (!imageUrl) {
    return (
      <Button
        aria-label={t('查看图片')}
        size='small'
        type='tertiary'
        theme='outline'
        icon={<Eye size={15} />}
        onClick={() => setRequestedKey(attachmentKey)}
      >
        {t('查看图片')} (
        {(Number(attachment?.size || 0) / 1024 / 1024).toFixed(2)} MiB)
      </Button>
    );
  }

  return (
    <>
      <button
        type='button'
        aria-label={t('查看图片')}
        className='block overflow-hidden rounded-md border border-[var(--semi-color-border)] bg-transparent'
        onClick={() => setPreviewVisible(true)}
      >
        <img
          src={imageUrl}
          alt={attachment.file_name || t('工单附件')}
          className='block max-h-48 max-w-full object-contain'
          loading='lazy'
        />
      </button>
      <ImagePreview
        src={imageUrl}
        visible={previewVisible}
        onVisibleChange={(visible) => {
          setPreviewVisible(visible);
          if (!visible) {
            setImageUrl('');
            setRequestedKey('');
          }
        }}
      />
    </>
  );
}
