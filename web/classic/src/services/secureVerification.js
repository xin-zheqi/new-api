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

import { API, showError } from '../helpers';
import {
  prepareCredentialRequestOptions,
  buildAssertionResult,
  isPasskeySupported,
} from '../helpers/passkey';
import i18n from '../i18n/i18n';

/**
 * 通用安全验证服务
 * 验证状态完全由后端 Session 控制，前端不存储任何状态
 */
export class SecureVerificationService {
  /**
   * 检查用户可用的验证方式
   * @returns {Promise<{has2FA: boolean, hasPasskey: boolean, passkeySupported: boolean}>}
   */
  static async checkAvailableVerificationMethods() {
    try {
      const [twoFAResponse, passkeyResponse, passkeySupported] =
        await Promise.all([
          API.get('/api/user/2fa/status'),
          API.get('/api/user/passkey'),
          isPasskeySupported(),
        ]);

      const has2FA =
        twoFAResponse.data?.success &&
        twoFAResponse.data?.data?.enabled === true;
      const hasPasskey =
        passkeyResponse.data?.success &&
        passkeyResponse.data?.data?.enabled === true;

      const result = {
        has2FA,
        hasPasskey,
        passkeySupported,
      };

      return result;
    } catch (error) {
      console.error('Failed to check verification methods:', error);
      return {
        has2FA: false,
        hasPasskey: false,
        passkeySupported: false,
      };
    }
  }

  /**
   * 执行2FA验证
   * @param {string} code - 验证码
   * @returns {Promise<void>}
   */
  static async verify2FA(scope, code) {
    if (!code?.trim()) {
      throw new Error(i18n.t('请输入验证码或备用码'));
    }

    // 调用通用验证 API，验证成功后后端会设置 session
    const verifyResponse = await API.post('/api/verify', {
      method: '2fa',
      code: code.trim(),
      scope,
    });

    if (!verifyResponse.data?.success) {
      throw new Error(
        verifyResponse.data?.message || i18n.t('验证失败，请重试'),
      );
    }

    const proof = verifyResponse.data?.data;
    if (!proof?.proof_token) {
      throw new Error(i18n.t('验证凭据未返回'));
    }
    return proof;
  }

  /**
   * 执行Passkey验证
   * @returns {Promise<void>}
   */
  static async verifyPasskey(scope) {
    try {
      // 开始Passkey验证
      const beginResponse = await API.post('/api/user/passkey/verify/begin', {
        scope,
      });
      if (!beginResponse.data?.success) {
        throw new Error(
          beginResponse.data?.message || i18n.t('开始验证失败'),
        );
      }

      // 准备WebAuthn选项
      const publicKey = prepareCredentialRequestOptions(
        beginResponse.data.data.options,
      );
      const flowToken = beginResponse.data.data.flow_token;
      if (!flowToken) {
        throw new Error(i18n.t('验证流程已过期'));
      }

      // 执行WebAuthn验证
      const credential = await navigator.credentials.get({ publicKey });
      if (!credential) {
        throw new Error(i18n.t('Passkey 验证被取消'));
      }

      // 构建验证结果
      const assertionResult = buildAssertionResult(credential);

      // 完成验证
      const finishResponse = await API.post('/api/user/passkey/verify/finish', {
        flow_token: flowToken,
        credential: assertionResult,
      });
      if (!finishResponse.data?.success) {
        throw new Error(
          finishResponse.data?.message || i18n.t('验证失败，请重试'),
        );
      }

      const proof = finishResponse.data?.data;
      if (!proof?.proof_token) {
        throw new Error(i18n.t('验证凭据未返回'));
      }
      return proof;
    } catch (error) {
      if (error.name === 'NotAllowedError') {
        throw new Error(i18n.t('Passkey 验证被取消或超时'));
      } else if (error.name === 'InvalidStateError') {
        throw new Error(i18n.t('Passkey 验证状态无效'));
      } else {
        throw error;
      }
    }
  }

  /**
   * 通用验证方法，根据验证类型执行相应的验证流程
   * @param {string} method - 验证方式: '2fa' | 'passkey'
   * @param {string} code - 2FA验证码（当method为'2fa'时必需）
   * @returns {Promise<void>}
   */
  static async verify(method, scope, code = '') {
    switch (method) {
      case '2fa':
        return await this.verify2FA(scope, code);
      case 'passkey':
        return await this.verifyPasskey(scope);
      default:
        throw new Error(i18n.t('不支持的验证方式: {{method}}', { method }));
    }
  }
}

/**
 * 预设的API调用函数工厂
 */
export const createApiCalls = {
  /**
   * 创建查看渠道密钥的API调用
   * @param {number} channelId - 渠道ID
   */
  viewChannelKey: (channelId) => async (proofToken) => {
    const response = await API.post(
      `/api/channel/${channelId}/key`,
      {},
      {
        headers: proofToken ? { 'X-Security-Proof': proofToken } : undefined,
        skipErrorHandler: true,
      },
    );
    return response.data;
  },

  /**
   * 创建自定义API调用
   * @param {string} url - API URL
   * @param {string} method - HTTP方法，默认为 'POST'
   * @param {Object} extraData - 额外的请求数据
   */
  custom:
    (url, method = 'POST', extraData = {}) =>
    async (proofToken) => {
      const data = extraData;
      const config = proofToken
        ? { headers: { 'X-Security-Proof': proofToken } }
        : {};

      let response;
      switch (method.toUpperCase()) {
        case 'GET':
          response = await API.get(url, { ...config, params: data });
          break;
        case 'POST':
          response = await API.post(url, data, config);
          break;
        case 'PUT':
          response = await API.put(url, data, config);
          break;
        case 'DELETE':
          response = await API.delete(url, { ...config, data });
          break;
        default:
          throw new Error(`不支持的HTTP方法: ${method}`);
      }
      return response.data;
    },
};
