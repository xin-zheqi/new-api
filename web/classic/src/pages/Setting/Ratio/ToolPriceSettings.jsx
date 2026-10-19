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
import React, { useEffect, useMemo, useState } from 'react';
import {
  Banner,
  Button,
  Input,
  InputNumber,
  Radio,
  RadioGroup,
  Table,
  TextArea,
  Typography,
} from '@douyinfe/semi-ui';
import { IconCopy, IconDelete, IconPlus } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { API, copy, showError, showSuccess } from '../../../helpers';

const { Text } = Typography;

const OPTION_KEY = 'tool_price_setting.prices';

const DEFAULT_PRICES = {
  web_search: 10.0,
  web_search_preview: 10.0,
  'web_search_preview:gpt-4o*': 25.0,
  'web_search_preview:gpt-4.1*': 25.0,
  'web_search_preview:gpt-4o-mini*': 25.0,
  'web_search_preview:gpt-4.1-mini*': 25.0,
  file_search: 2.5,
  google_search: 14.0,
  'google_search:gemini-2.5*': 0.0,
  'google_search:gemini-2.0*': 0.0,
  'google_search:gemini-1.5*': 0.0,
  'google_search_grounded_prompt:gemini-2.5*': 35.0,
  'google_search_grounded_prompt:gemini-2.0*': 35.0,
  'google_search_grounded_prompt:gemini-1.5*': 35.0,
  image_generation: 150.0,
  search_std: 10.0,
  search_pro: 30.0,
  search_pro_sogou: 50.0,
  search_pro_quark: 50.0,
  search_strategy_turbo: 3.0,
  search_strategy_max: 4.0,
  search_strategy_agent: 4.0,
  search_strategy_agent_max: 4.0,
  bing_web_search: 14.0,
  'web_search:grok*': 5.0,
  x_search_posts: 5.0,
  x_search_profiles: 10.0,
  deepseek_search: 0.75,
  deepseek_fetch: 0.75,
};

function rowsToObject(rows) {
  const prices = {};
  for (const row of rows) {
    const k = row.key.trim();
    if (!k) continue;
    prices[k] = Number(row.price);
  }
  return prices;
}

function objectToRows(prices) {
  return Object.entries(prices).map(([key, price], i) => ({
    id: i,
    key,
    price,
  }));
}

export default function ToolPriceSettings({ options }) {
  const { t } = useTranslation();
  const [rows, setRows] = useState([]);
  const [mode, setMode] = useState('visual');
  const [jsonText, setJsonText] = useState('');
  const [jsonError, setJsonError] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    let prices = {};
    try {
      const raw = options?.[OPTION_KEY];
      if (raw) {
        prices = typeof raw === 'string' ? JSON.parse(raw) : raw;
      }
    } catch {
      prices = {};
    }

    if (!prices || Object.keys(prices).length === 0) {
      prices = { ...DEFAULT_PRICES };
    }

    // Newly supported tools appear without replacing explicit custom/zero prices.
    prices = { ...DEFAULT_PRICES, ...prices };
    setRows(objectToRows(prices));
    setJsonText(JSON.stringify(prices, null, 2));
  }, [options]);

  const syncToJson = (nextRows) => {
    setRows(nextRows);
    setJsonText(JSON.stringify(rowsToObject(nextRows), null, 2));
    setJsonError('');
  };

  const syncToVisual = (text) => {
    setJsonText(text);
    try {
      const parsed = JSON.parse(text);
      if (
        typeof parsed !== 'object' ||
        Array.isArray(parsed) ||
        parsed === null
      ) {
        setJsonError(t('JSON must be an object'));
        return;
      }
      if (
        Object.values(parsed).some(
          (value) =>
            typeof value !== 'number' || !Number.isFinite(value) || value < 0,
        )
      ) {
        setJsonError(t('Tool prices must be finite non-negative numbers'));
        return;
      }
      setRows(objectToRows(parsed));
      setJsonError('');
    } catch (e) {
      setJsonError(e.message);
    }
  };

  const updateRow = (id, field, value) => {
    syncToJson(rows.map((r) => (r.id === id ? { ...r, [field]: value } : r)));
  };

  const addRow = () => {
    syncToJson([...rows, { id: Date.now(), key: '', price: 0 }]);
  };

  const removeRow = (id) => {
    syncToJson(rows.filter((r) => r.id !== id));
  };

  const resetToDefault = () => {
    syncToJson(objectToRows(DEFAULT_PRICES));
  };

  const currentPrices = useMemo(() => rowsToObject(rows), [rows]);

  const handleSave = async () => {
    if (
      jsonError ||
      rows.some(
        (row) =>
          row.key.trim() &&
          (row.price === '' ||
            !Number.isFinite(Number(row.price)) ||
            Number(row.price) < 0),
      )
    ) {
      showError(t('Tool prices must be finite non-negative numbers'));
      return;
    }
    setSaving(true);
    try {
      const res = await API.put('/api/option/', {
        key: OPTION_KEY,
        value: JSON.stringify(currentPrices),
      });
      if (res.data.success) {
        showSuccess(t('保存成功'));
      } else {
        showError(res.data.message || t('保存失败'));
      }
    } catch (e) {
      showError(e.message);
    } finally {
      setSaving(false);
    }
  };

  const columns = [
    {
      title: t('Tool identifier'),
      dataIndex: 'key',
      render: (text, record) => (
        <Input
          value={text}
          placeholder='web_search_preview:gpt-4o*'
          onChange={(val) => updateRow(record.id, 'key', val)}
          style={{ width: '100%' }}
        />
      ),
    },
    {
      title: t('价格') + ' ($/1K' + t('次') + ')',
      dataIndex: 'price',
      width: 160,
      render: (val, record) => (
        <InputNumber
          value={val}
          min={0}
          step={0.5}
          onChange={(v) => updateRow(record.id, 'price', v ?? '')}
          style={{ width: '100%' }}
        />
      ),
    },
    {
      title: t('操作'),
      width: 60,
      render: (_, record) => (
        <Button
          icon={<IconDelete />}
          type='danger'
          theme='borderless'
          size='small'
          onClick={() => removeRow(record.id)}
        />
      ),
    },
  ];

  return (
    <div style={{ maxWidth: 700 }}>
      <Banner
        type='info'
        description={
          <>
            <div>
              {t(
                'Configure per-tool unit prices ($/1K calls). Per-request models do not incur additional tool fees.',
              )}
            </div>
            <div style={{ marginTop: 4 }}>
              <Text strong>{t('Format')}：</Text>
              <code>web_search_preview</code> {t('is the default price; ')}，
              <code>web_search_preview:gpt-4o*</code>{' '}
              {t('overrides for matching model prefix.')}
            </div>
          </>
        }
        style={{ marginBottom: 16 }}
      />

      <RadioGroup
        type='button'
        size='small'
        value={mode}
        onChange={(e) => setMode(e.target.value)}
        style={{ marginBottom: 12 }}
      >
        <Radio value='visual'>{t('可视化')}</Radio>
        <Radio value='json'>JSON</Radio>
      </RadioGroup>

      {mode === 'visual' ? (
        <>
          <Table
            dataSource={rows}
            columns={columns}
            pagination={false}
            size='small'
            rowKey='id'
          />
          <div style={{ display: 'flex', gap: 8, marginTop: 12 }}>
            <Button icon={<IconPlus />} onClick={addRow}>
              {t('添加')}
            </Button>
            <Button theme='borderless' onClick={resetToDefault}>
              {t('Restore defaults')}
            </Button>
          </div>
        </>
      ) : (
        <>
          <TextArea
            value={jsonText}
            onChange={syncToVisual}
            autosize={{ minRows: 8, maxRows: 20 }}
            style={{ fontFamily: 'monospace', fontSize: 13 }}
          />
          {jsonError && (
            <Text
              type='danger'
              size='small'
              style={{ display: 'block', marginTop: 4 }}
            >
              {jsonError}
            </Text>
          )}
          <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
            <Button
              icon={<IconCopy />}
              size='small'
              theme='borderless'
              onClick={() => {
                copy(jsonText, t('JSON'));
              }}
            >
              {t('复制')}
            </Button>
            <Button size='small' theme='borderless' onClick={resetToDefault}>
              {t('Restore defaults')}
            </Button>
          </div>
        </>
      )}

      <div
        style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 16 }}
      >
        <Button
          theme='solid'
          type='primary'
          loading={saving}
          disabled={mode === 'json' && !!jsonError}
          onClick={handleSave}
        >
          {t('保存')}
        </Button>
      </div>
    </div>
  );
}
