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

import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { reducer, initialState } from './reducer';
import { normalizeLanguage } from '../../i18n/language';
import { bootstrapAuthentication } from '../../helpers/authSession';

export const UserContext = React.createContext({
  state: initialState,
  dispatch: () => null,
});

export const UserProvider = ({ children }) => {
  const [state, dispatch] = React.useReducer(reducer, initialState);
  const [authReady, setAuthReady] = useState(false);
  const { i18n } = useTranslation();

  useEffect(() => {
    let active = true;
    bootstrapAuthentication()
      .then((user) => {
        if (active && user) dispatch({ type: 'login', payload: user });
      })
      .finally(() => {
        if (active) setAuthReady(true);
      });
    return () => {
      active = false;
    };
  }, []);

  // Sync language preference when user data is loaded
  useEffect(() => {
    if (state.user?.setting) {
      try {
        const settings = JSON.parse(state.user.setting);
        const normalizedLanguage = normalizeLanguage(settings.language);
        if (normalizedLanguage && normalizedLanguage !== i18n.language) {
          i18n.changeLanguage(normalizedLanguage);
        }
        if (normalizedLanguage) {
          localStorage.setItem('i18nextLng', normalizedLanguage);
        }
      } catch (e) {
        // Ignore parse errors
      }
    }
  }, [state.user?.setting, i18n]);

  return authReady ? (
    <UserContext.Provider value={[state, dispatch]}>
      {children}
    </UserContext.Provider>
  ) : null;
};
