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
import React from 'react';
import { Button, TextArea } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';

// Templates only append routes to the draft. Never replace existing custom
// authentication, model filters or unknown fields in the stored configuration.
export default function AdvancedCustomRoutes({ value, onChange }) {
  const { t } = useTranslation();
  let config;
  try {
    config = JSON.parse(value);
  } catch {
    config = null;
  }
  const valid =
    config && !Array.isArray(config) && Array.isArray(config.advanced_routes);
  const add = (incoming, converter) =>
    onChange(
      JSON.stringify(
        {
          ...config,
          advanced_routes: [
            ...config.advanced_routes,
            {
              incoming_path: incoming,
              upstream_path: '/v1/responses',
              converter,
              auth: {
                type: 'header',
                name: 'Authorization',
                value: 'Bearer {api_key}',
              },
            },
          ],
        },
        null,
        2,
      ),
    );
  return (
    <div className='space-y-2 my-4'>
      <label htmlFor='classic-advanced-routes'>{t('Advanced routes')}</label>
      <TextArea
        id='classic-advanced-routes'
        value={value}
        onChange={onChange}
        autosize={{ minRows: 8, maxRows: 20 }}
      />
      {!valid && <p role='alert'>{t('Advanced routes JSON is invalid')}</p>}
      <div className='flex flex-wrap gap-2'>
        <Button
          disabled={!valid}
          onClick={() =>
            add('/v1/messages', 'claude_messages_to_openai_responses')
          }
        >
          {t('Anthropic Messages to OpenAI Responses')}
        </Button>
        <Button
          disabled={!valid}
          onClick={() =>
            add(
              '/v1beta/models/{model}:generateContent',
              'gemini_generate_content_to_openai_responses',
            )
          }
        >
          {t('Gemini Generate Content to OpenAI Responses')}
        </Button>
      </div>
    </div>
  );
}
