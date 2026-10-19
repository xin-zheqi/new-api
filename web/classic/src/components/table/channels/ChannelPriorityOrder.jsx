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
import { Banner, Button, Modal, Spin } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API } from '../../../helpers/api';

// Always use the server's complete order. Sorting a filtered page locally would
// accidentally assign priorities without considering channels on other pages.
export function ChannelPriorityEditor({ channels = [], onSaved }) {
  const { t } = useTranslation();
  const [order, setOrder] = useState(null);
  const [preview, setPreview] = useState(null);
  const [undo, setUndo] = useState(null);
  const [moving, setMoving] = useState('');
  const [target, setTarget] = useState('');
  const [position, setPosition] = useState('before');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const requestBusy = useRef(false);
  const dragged = useRef(null);
  useEffect(() => {
    let active = true;
    API.get('/api/channel/priority/order')
      .then(({ data }) => {
        if (!data.success)
          throw new Error(data.message || 'Failed to load settings');
        if (active) setOrder(data.data);
      })
      .catch((err) => {
        if (active) setError(err.message);
      });
    return () => {
      active = false;
    };
  }, []);
  const names = new Map(channels.map((channel) => [channel.id, channel.name]));
  const name = (id) => (names.has(id) ? `#${id} ${names.get(id)}` : `#${id}`);

  const move = async (payload, isPreview = false) => {
    if (!order || requestBusy.current) return;
    requestBusy.current = true;
    setBusy(true);
    setError('');
    try {
      const { data } = await API.post('/api/channel/priority/reorder', {
        ...payload,
        preview: isPreview,
      });
      if (!data.success)
        throw new Error(data.message || 'Failed to update channel');
      if (isPreview) {
        setPreview({ order: data.data, payload });
      } else {
        setOrder(data.data);
        setPreview(null);
        setUndo(payload.restore ? null : data.data);
        onSaved?.();
      }
    } catch (err) {
      setError(err.message);
      setPreview(null);
      setUndo(null);
      // A stale revision must not be retried automatically; refresh for the next
      // deliberate move and leave the conflict visible to the administrator.
      try {
        const { data } = await API.get('/api/channel/priority/order');
        if (data.success) setOrder(data.data);
      } catch {
        /* Keep the original failure visible if refresh also fails. */
      }
    } finally {
      requestBusy.current = false;
      setBusy(false);
    }
  };
  const prepare = (id, targetId, relation) => {
    if (id === targetId || !id || !targetId) return;
    void move(
      { id, target_id: targetId, position: relation, revision: order.revision },
      true,
    );
  };
  const displayed = preview?.order ?? order;
  return (
    <div className='space-y-3'>
      <p>
        {t(
          'Move channels into a priority tier or between tiers. Priority values update automatically.',
        )}
      </p>
      <p>
        {t(
          'All channels are shown here; filters and pagination do not limit priority changes.',
        )}
      </p>
      {error && (
        <div>
          <Banner type='danger' description={t(error)} closeIcon={null} />
        </div>
      )}
      {!order && !error && <Spin />}
      {order && (
        <>
          <fieldset
            disabled={busy || !!preview}
            className='flex flex-wrap gap-2'
          >
            <label>
              {t('Channel')}
              <select
                aria-label={t('Channel')}
                className='bg-transparent border rounded p-2'
                value={moving}
                onChange={(event) => setMoving(event.target.value)}
              >
                <option value=''>{t('Select a channel')}</option>
                {order.items.map((item) => (
                  <option key={item.id} value={item.id}>
                    {name(item.id)}
                  </option>
                ))}
              </select>
            </label>
            <label>
              {t('Position')}
              <select
                aria-label={t('Position')}
                className='bg-transparent border rounded p-2'
                value={position}
                onChange={(event) => setPosition(event.target.value)}
              >
                <option value='before'>{t('Before')}</option>
                <option value='same'>{t('Same priority')}</option>
                <option value='after'>{t('After')}</option>
              </select>
            </label>
            <label>
              {t('Target channel')}
              <select
                aria-label={t('Target channel')}
                className='bg-transparent border rounded p-2'
                value={target}
                onChange={(event) => setTarget(event.target.value)}
              >
                <option value=''>{t('Select a channel')}</option>
                {order.items.map((item) => (
                  <option key={item.id} value={item.id}>
                    {name(item.id)}
                  </option>
                ))}
              </select>
            </label>
            <Button
              disabled={
                busy || !!preview || !moving || !target || moving === target
              }
              onClick={() => prepare(Number(moving), Number(target), position)}
            >
              {t('Preview')}
            </Button>
          </fieldset>
          <div
            role='region'
            aria-label={t('Channel priorities')}
            tabIndex={0}
            className='overflow-auto max-h-96 border rounded'
          >
            <table className='w-full text-left'>
              <thead>
                <tr>
                  <th className='p-2'>{t('Channel')}</th>
                  <th className='p-2'>{t('Priority')}</th>
                </tr>
              </thead>
              <tbody>
                {displayed.items.map((item) => (
                  <tr
                    key={item.id}
                    onDragOver={(event) => {
                      if (!busy && !preview) event.preventDefault();
                    }}
                    onDrop={(event) => {
                      event.preventDefault();
                      if (busy || preview || !dragged.current) return;
                      const bounds =
                        event.currentTarget.getBoundingClientRect();
                      const fraction =
                        (event.clientY - bounds.top) / bounds.height;
                      const relation =
                        fraction < 0.25
                          ? 'before'
                          : fraction > 0.75
                            ? 'after'
                            : 'same';
                      prepare(dragged.current, item.id, relation);
                      dragged.current = null;
                    }}
                  >
                    <td className='p-2 break-all'>
                      <button
                        type='button'
                        disabled={busy || !!preview}
                        draggable={!busy && !preview}
                        aria-label={`${t('Move channel')} ${name(item.id)}`}
                        className='cursor-grab p-1 mr-2'
                        onClick={() => setMoving(String(item.id))}
                        onDragStart={(event) => {
                          dragged.current = item.id;
                          event.dataTransfer.setData(
                            'text/plain',
                            String(item.id),
                          );
                          event.dataTransfer.effectAllowed = 'move';
                        }}
                        onDragEnd={() => {
                          dragged.current = null;
                        }}
                      >
                        ↕
                      </button>
                      {name(item.id)}
                    </td>
                    <td className='p-2'>{item.priority ?? 0}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            {!displayed.items.length && <p>{t('No channels available')}</p>}
          </div>
          {preview && (
            <div role='status' className='space-y-2'>
              <p>
                {t('Priority changes')}: {preview.order.changes.length}
              </p>
              <ul>
                {preview.order.changes.map((change) => (
                  <li key={change.id}>
                    {name(change.id)}: {change.previous ?? 0} →{' '}
                    {change.priority ?? 0}
                  </li>
                ))}
              </ul>
              <Button loading={busy} onClick={() => void move(preview.payload)}>
                {t('Save Changes')}
              </Button>
              <Button disabled={busy} onClick={() => setPreview(null)}>
                {t('Cancel')}
              </Button>
            </div>
          )}
          {undo && (
            <Button
              disabled={busy || !!preview || !undo.changes.length}
              onClick={() =>
                void move({
                  revision: undo.revision,
                  restore: undo.changes.map((change) => ({
                    id: change.id,
                    previous: change.priority,
                    priority: change.previous,
                  })),
                })
              }
            >
              {t('Undo last move')}
            </Button>
          )}
        </>
      )}
    </div>
  );
}

export default function ChannelPriorityOrder({ channels, refresh }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button onClick={() => setOpen(true)}>
        {t('Drag to set channel priority')}
      </Button>
      <Modal
        title={t('Drag to set channel priority')}
        visible={open}
        footer={null}
        style={{ maxWidth: '95vw', width: 760 }}
        onCancel={() => {
          setOpen(false);
          void refresh();
        }}
      >
        {open && (
          <ChannelPriorityEditor channels={channels} onSaved={refresh} />
        )}
      </Modal>
    </>
  );
}
