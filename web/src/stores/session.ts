// The signed-in account, if any. The session itself is an HttpOnly cookie
// the SPA cannot read; this store only mirrors /auth/me.
import m from 'mithril';
import { ApiError, setCSRFToken } from '../api/http';
import { reader } from '../api/reader';

export interface User {
  id: string;
  email: string;
  handle: string | null;
  display_name: string;
  role: 'reader' | 'author' | 'moderator' | 'owner';
  email_verified: boolean;
  timezone: string;
  settings: Record<string, unknown> | null;
  permissions: { can_comment: boolean; can_review: boolean; studio: boolean; admin: boolean };
}

interface SessionPayload {
  user: User;
  csrf_token: string;
}

let loaded: Promise<void> | null = null;

export const session = {
  user: null as User | null,
  ready: false,

  /** Loads /auth/me once; anonymous visitors simply get user = null. */
  load(): Promise<void> {
    if (!loaded) {
      loaded = reader
        .get<SessionPayload>('/auth/me')
        .then((p) => this.accept(p))
        .catch((e) => {
          if (!(e instanceof ApiError) || e.status !== 401) console.warn('session check failed', e);
          this.user = null;
        })
        .finally(() => {
          this.ready = true;
          m.redraw();
        });
    }
    return loaded;
  },

  accept(p: SessionPayload): void {
    this.user = p.user;
    setCSRFToken(p.csrf_token);
  },

  async login(email: string, password: string): Promise<void> {
    this.accept(await reader.post<SessionPayload>('/auth/login', { email, password }));
  },

  async logout(all = false): Promise<void> {
    try {
      await reader.post(all ? '/auth/logout-all' : '/auth/logout');
    } finally {
      this.user = null;
      setCSRFToken('');
    }
  },

  async refresh(): Promise<void> {
    loaded = null;
    await this.load();
  },
};
