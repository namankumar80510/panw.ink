// Sign-in, registration, email verification and password reset (§4.2).
import m from 'mithril';
import { ApiError } from '../api/http';
import { reader } from '../api/reader';
import { alertBox, errorText, field, pageHeader, submitButton } from '../components/ui';
import { toast } from '../components/toasts';
import { config } from '../stores/config';
import { session } from '../stores/session';
import { safeNext } from '../utils/redirect';
import { setTitle } from '../utils/title';

function nextParam(): string {
  return safeNext(m.route.param('next'));
}

/** Reads a one-time token from the URL, then removes it from the address bar and history. */
function takeToken(): string {
  const token = m.route.param('token') ?? '';
  if (token) window.history.replaceState(window.history.state, '', window.location.pathname);
  return token;
}

export function LoginPage(): m.Component {
  const s = { email: '', password: '', busy: false, error: '' };
  return {
    oninit: () => {
      setTitle('Sign in');
      void session.load().then(() => { if (session.user) m.route.set(nextParam(), {}, { replace: true }); });
    },
    view: () =>
      m('.page-narrow.mx-auto', [
        pageHeader('Sign in', 'Welcome back. Reading never requires an account; signing in syncs your library and progress.'),
        s.error ? alertBox('danger', s.error) : null,
        m('form', {
          novalidate: true,
          onsubmit: async (e: Event) => {
            e.preventDefault();
            s.busy = true; s.error = '';
            try {
              await session.login(s.email, s.password);
              toast('Signed in.', 'success');
              m.route.set(nextParam());
            } catch (err) {
              s.error = errorText(err);
            } finally {
              s.busy = false; s.password = '';
            }
          },
        }, [
          field({ id: 'email', label: 'Email', type: 'email', value: s.email, oninput: (v) => (s.email = v), autocomplete: 'email', required: true }),
          field({ id: 'password', label: 'Password', type: 'password', value: s.password, oninput: (v) => (s.password = v), autocomplete: 'current-password', required: true }),
          m('.d-flex.flex-wrap.gap-3.align-items-center', [
            submitButton('Sign in', s.busy),
            m(m.route.Link, { href: '/forgot-password' }, 'Forgot your password?'),
          ]),
        ]),
        m('hr.my-4'),
        m('p', ['New here? ', m(m.route.Link, { href: '/register' + (m.route.param('next') ? '?next=' + encodeURIComponent(nextParam()) : '') }, 'Create a reader account'), '.']),
      ]),
  };
}

export function RegisterPage(): m.Component {
  const s = { email: '', display: '', handle: '', password: '', busy: false, error: '', fields: {} as Record<string, string>, done: false, message: '' };
  return {
    oninit: () => setTitle('Create account'),
    view: () =>
      m('.page-narrow.mx-auto', s.done
        ? [
            pageHeader('Check your inbox'),
            alertBox('success', s.message),
            m('p', 'Open the link in the email to confirm your address. You can sign in now and verify later; commenting, reviewing and Patreon linking need a verified email.'),
            m(m.route.Link, { href: '/login', class: 'btn btn-primary' }, 'Go to sign in'),
          ]
        : [
            pageHeader('Create a reader account', 'Save stories, keep your place across devices and join the discussion.'),
            s.error ? alertBox('danger', s.error) : null,
            m('form', {
              novalidate: true,
              onsubmit: async (e: Event) => {
                e.preventDefault();
                s.busy = true; s.error = ''; s.fields = {};
                try {
                  const res = await reader.post<{ message: string }>('/auth/register', {
                    email: s.email, password: s.password, display_name: s.display, handle: s.handle || null,
                  });
                  s.done = true; s.message = res.message;
                } catch (err) {
                  if (err instanceof ApiError && Object.keys(err.fields).length) s.fields = err.fields;
                  else s.error = errorText(err);
                } finally {
                  s.busy = false; s.password = '';
                }
              },
            }, [
              field({ id: 'email', label: 'Email', type: 'email', value: s.email, oninput: (v) => (s.email = v), autocomplete: 'email', required: true, error: s.fields.email, help: 'Never shown publicly.' }),
              field({ id: 'display_name', label: 'Display name', value: s.display, oninput: (v) => (s.display = v), autocomplete: 'nickname', required: true, maxlength: 80, error: s.fields.display_name, help: 'Shown with your reviews and comments.' }),
              field({ id: 'handle', label: 'Handle (optional)', value: s.handle, oninput: (v) => (s.handle = v.toLowerCase()), maxlength: 40, error: s.fields.handle, help: '3–40 lowercase letters, digits or underscores.' }),
              field({ id: 'password', label: 'Password', type: 'password', value: s.password, oninput: (v) => (s.password = v), autocomplete: 'new-password', required: true, minlength: config.passwordMin, error: s.fields.password, help: `At least ${config.passwordMin} characters.` }),
              submitButton('Create account', s.busy),
            ]),
            m('p.mt-4', ['Already have an account? ', m(m.route.Link, { href: '/login' }, 'Sign in'), '.']),
          ]),
  };
}

export function VerifyEmailPage(): m.Component {
  const s = { state: 'working' as 'working' | 'ok' | 'error' | 'none', error: '', email: '', resent: false, busy: false };
  return {
    oninit: async () => {
      setTitle('Verify email');
      const token = takeToken();
      if (!token) { s.state = 'none'; return; }
      try {
        await reader.post('/auth/verify-email', { token });
        s.state = 'ok';
        if (session.user) await session.refresh();
      } catch (err) {
        s.state = 'error'; s.error = errorText(err);
      }
      m.redraw();
    },
    view: () =>
      m('.page-narrow.mx-auto', [
        pageHeader('Verify your email'),
        s.state === 'working' ? m('p', { role: 'status' }, 'Confirming…') : null,
        s.state === 'ok' ? [alertBox('success', 'Your email address is confirmed. Thank you!'), m(m.route.Link, { href: session.user ? '/' : '/login', class: 'btn btn-primary' }, session.user ? 'Continue reading' : 'Sign in')] : null,
        s.state === 'error' || s.state === 'none'
          ? [
              s.state === 'error' ? alertBox('danger', s.error) : m('p', 'Use the link from your verification email, or request a new one below.'),
              s.resent
                ? alertBox('info', 'If that address has an unverified account, a new link is on its way.')
                : m('form', {
                    onsubmit: async (e: Event) => {
                      e.preventDefault(); s.busy = true;
                      try { await reader.post('/auth/resend-verification', { email: s.email }); s.resent = true; } catch (err) { toast(errorText(err), 'danger'); } finally { s.busy = false; }
                    },
                  }, [field({ id: 'email', label: 'Email', type: 'email', value: s.email, oninput: (v) => (s.email = v), autocomplete: 'email', required: true }), submitButton('Send a new link', s.busy, 'btn-outline-primary')]),
            ]
          : null,
      ]),
  };
}

export function ForgotPasswordPage(): m.Component {
  const s = { email: '', busy: false, sent: false, error: '' };
  return {
    oninit: () => setTitle('Forgot password'),
    view: () =>
      m('.page-narrow.mx-auto', [
        pageHeader('Reset your password', 'Enter your account email and we will send a reset link.'),
        s.error ? alertBox('danger', s.error) : null,
        s.sent
          ? alertBox('info', 'If an account matches that address, we have sent it an email. The link works once and expires in an hour.')
          : m('form', {
              onsubmit: async (e: Event) => {
                e.preventDefault(); s.busy = true; s.error = '';
                try { await reader.post('/auth/password/forgot', { email: s.email }); s.sent = true; } catch (err) { s.error = errorText(err); } finally { s.busy = false; }
              },
            }, [field({ id: 'email', label: 'Email', type: 'email', value: s.email, oninput: (v) => (s.email = v), autocomplete: 'email', required: true }), submitButton('Send reset link', s.busy)]),
      ]),
  };
}

export function ResetPasswordPage(): m.Component {
  const s = { token: '', pw: '', pw2: '', busy: false, done: false, error: '', fields: {} as Record<string, string> };
  return {
    oninit: () => { setTitle('Choose a new password'); s.token = takeToken(); },
    view: () =>
      m('.page-narrow.mx-auto', [
        pageHeader('Choose a new password'),
        s.done
          ? [alertBox('success', 'Your password was changed and all sessions were signed out.'), m(m.route.Link, { href: '/login', class: 'btn btn-primary' }, 'Sign in')]
          : !s.token
            ? [alertBox('warning', 'This page needs the link from your reset email.'), m(m.route.Link, { href: '/forgot-password' }, 'Request a new link')]
            : [
                s.error ? alertBox('danger', s.error) : null,
                m('form', {
                  novalidate: true,
                  onsubmit: async (e: Event) => {
                    e.preventDefault(); s.error = ''; s.fields = {};
                    if (s.pw !== s.pw2) { s.fields = { new_password2: 'Passwords do not match.' }; return; }
                    s.busy = true;
                    try {
                      await reader.post('/auth/password/reset', { token: s.token, new_password: s.pw });
                      s.done = true; s.token = ''; session.user = null;
                    } catch (err) {
                      if (err instanceof ApiError && Object.keys(err.fields).length) s.fields = err.fields; else s.error = errorText(err);
                    } finally { s.busy = false; }
                  },
                }, [
                  field({ id: 'new_password', label: 'New password', type: 'password', value: s.pw, oninput: (v) => (s.pw = v), autocomplete: 'new-password', required: true, minlength: config.passwordMin, error: s.fields.new_password, help: `At least ${config.passwordMin} characters.` }),
                  field({ id: 'new_password2', label: 'Repeat new password', type: 'password', value: s.pw2, oninput: (v) => (s.pw2 = v), autocomplete: 'new-password', required: true, error: s.fields.new_password2 }),
                  submitButton('Set password', s.busy),
                ]),
              ],
      ]),
  };
}
