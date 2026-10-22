// Reader account pages (§4.2). Library/history/notifications gain data in
// Phase 4; settings covers profile, password, sessions and reading prefs.
import m from 'mithril';
import { ApiError } from '../api/http';
import { reader } from '../api/reader';
import { alertBox, emptyState, errorText, field, pageHeader, spinner, submitButton } from '../components/ui';
import { toast } from '../components/toasts';
import { config } from '../stores/config';
import { prefs, type Theme } from '../stores/prefs';
import { session, type User } from '../stores/session';
import { setTitle } from '../utils/title';

interface SessionInfo {
  id: string;
  created_at: string;
  last_seen_at: string;
  device: string;
  current: boolean;
}

function fmt(ts: string, tz?: string): string {
  try {
    return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short', timeZone: tz }).format(new Date(ts));
  } catch {
    return ts;
  }
}

/** IANA zones for the picker. Browsers omit "UTC" from supportedValuesOf, but it is the account default. */
function timeZones(current: string): string[] {
  const fn = (Intl as unknown as { supportedValuesOf?: (k: string) => string[] }).supportedValuesOf;
  const zones = fn ? fn('timeZone') : [];
  return ['UTC', ...zones.filter((z) => z !== 'UTC'), ...(current && current !== 'UTC' && !zones.includes(current) ? [current] : [])];
}

function ProfileCard(): m.Component<{ user: User }> {
  const s = { display: '', tz: '', busy: false, fields: {} as Record<string, string> };
  return {
    oninit: ({ attrs }) => { s.display = attrs.user.display_name; s.tz = attrs.user.timezone; },
    view: ({ attrs: { user } }) =>
      m('section.card.mb-4', m('.card-body', [
        m('h2.h5.card-title', 'Profile'),
        m('p.small.text-body-secondary', ['Signed in as ', m('strong', user.email), user.handle ? ` (@${user.handle})` : '', '. Your email is never shown publicly.']),
        !user.email_verified
          ? alertBox('warning', ['Your email address is not verified yet. ', m(m.route.Link, { href: '/verify-email' }, 'Resend the verification link'), '.'])
          : null,
        m('form', {
          onsubmit: async (e: Event) => {
            e.preventDefault(); s.busy = true; s.fields = {};
            try {
              session.user = await reader.patch<User>('/users/me', { display_name: s.display, timezone: s.tz });
              toast('Profile saved.', 'success');
            } catch (err) {
              if (err instanceof ApiError && Object.keys(err.fields).length) s.fields = err.fields; else toast(errorText(err), 'danger');
            } finally { s.busy = false; }
          },
        }, [
          field({ id: 'display_name', label: 'Display name', value: s.display, oninput: (v) => (s.display = v), maxlength: 80, required: true, error: s.fields.display_name }),
          m('.mb-3', [
            m('label.form-label', { for: 'timezone' }, 'Time zone'),
            m('select.form-select', { id: 'timezone', value: s.tz, onchange: (e: Event) => (s.tz = (e.target as HTMLSelectElement).value), class: s.fields.timezone ? 'is-invalid' : '' },
              timeZones(s.tz).map((z) => m('option', { value: z, selected: z === s.tz }, z))),
            m('.form-text', 'Release dates are shown in this time zone. The server always schedules in UTC.'),
          ]),
          submitButton('Save profile', s.busy),
        ]),
      ])),
  };
}

function PasswordCard(): m.Component {
  const s = { old: '', pw: '', busy: false, fields: {} as Record<string, string> };
  return {
    view: () =>
      m('section.card.mb-4', m('.card-body', [
        m('h2.h5.card-title', 'Change password'),
        m('p.small.text-body-secondary', 'Changing your password signs out every other device.'),
        m('form', {
          novalidate: true,
          onsubmit: async (e: Event) => {
            e.preventDefault(); s.busy = true; s.fields = {};
            try {
              await reader.post('/auth/password/change', { old_password: s.old, new_password: s.pw });
              await session.refresh();
              s.old = ''; s.pw = '';
              toast('Password changed. Other sessions were signed out.', 'success');
            } catch (err) {
              if (err instanceof ApiError && Object.keys(err.fields).length) s.fields = err.fields; else toast(errorText(err), 'danger');
            } finally { s.busy = false; }
          },
        }, [
          field({ id: 'old_password', label: 'Current password', type: 'password', value: s.old, oninput: (v) => (s.old = v), autocomplete: 'current-password', required: true, error: s.fields.old_password }),
          field({ id: 'new_password', label: 'New password', type: 'password', value: s.pw, oninput: (v) => (s.pw = v), autocomplete: 'new-password', required: true, minlength: config.passwordMin, error: s.fields.new_password, help: `At least ${config.passwordMin} characters.` }),
          submitButton('Change password', s.busy, 'btn-outline-primary'),
        ]),
      ])),
  };
}

function SessionsCard(): m.Component<{ user: User }> {
  let items: SessionInfo[] | null = null;
  let error = '';
  const load = async () => {
    try { items = await reader.get<SessionInfo[]>('/users/me/sessions'); } catch (e) { error = errorText(e); }
    m.redraw();
  };
  return {
    oninit: load,
    view: ({ attrs: { user } }) =>
      m('section.card.mb-4', m('.card-body', [
        m('h2.h5.card-title', 'Signed-in devices'),
        error ? alertBox('danger', error) : items === null ? spinner() : m('ul.list-group.mb-3', items.map((it) =>
          m('li.list-group-item.d-flex.justify-content-between.align-items-center.gap-2', [
            m('div', [m('div', [it.device, it.current ? m('span.badge.text-bg-success.ms-2', 'This device') : null]),
              m('small.text-body-secondary', `Signed in ${fmt(it.created_at, user.timezone)} · last active ${fmt(it.last_seen_at, user.timezone)}`)]),
            it.current ? null : m('button.btn.btn-sm.btn-outline-danger', {
              type: 'button',
              onclick: async () => { try { await reader.del('/users/me/sessions/' + it.id); await load(); } catch (e) { toast(errorText(e), 'danger'); } },
            }, 'Sign out'),
          ]))),
        m('button.btn.btn-outline-danger', {
          type: 'button',
          onclick: async () => {
            await session.logout(true);
            toast('Signed out on every device.', 'success');
            m.route.set('/');
          },
        }, 'Sign out everywhere'),
      ])),
  };
}

const PrefsCard: m.Component = {
  view: () => {
    const p = prefs.value;
    const range = (id: string, label: string, min: number, max: number, step: number, value: number, set: (n: number) => void, unit = '') =>
      m('.mb-3', [
        m('label.form-label', { for: id }, `${label}: ${value}${unit}`),
        m('input.form-range', { id, type: 'range', min, max, step, value, oninput: (e: Event) => set(Number((e.target as HTMLInputElement).value)) }),
      ]);
    return m('section.card.mb-4', m('.card-body', [
      m('h2.h5.card-title', 'Reading preferences'),
      m('p.small.text-body-secondary', 'Saved in this browser.'),
      m('fieldset.mb-3', [
        m('legend.form-label.fs-6', 'Theme'),
        (['auto', 'light', 'dark', 'sepia'] as Theme[]).map((t) =>
          m('.form-check.form-check-inline', [
            m('input.form-check-input', { type: 'radio', name: 'theme', id: 'theme-' + t, checked: p.theme === t, onchange: () => prefs.set({ theme: t }) }),
            m('label.form-check-label', { for: 'theme-' + t }, t === 'auto' ? 'Match device' : t[0]!.toUpperCase() + t.slice(1)),
          ])),
      ]),
      range('font-size', 'Text size', 14, 26, 1, p.fontSize, (n) => prefs.set({ fontSize: n }), 'px'),
      range('line-height', 'Line spacing', 1.45, 2, 0.05, p.lineHeight, (n) => prefs.set({ lineHeight: n })),
      range('width', 'Text width', 560, 900, 20, p.width, (n) => prefs.set({ width: n }), 'px'),
      m('.mb-3', [
        m('label.form-label', { for: 'font-family' }, 'Typeface'),
        m('select.form-select', { id: 'font-family', value: p.fontFamily, onchange: (e: Event) => prefs.set({ fontFamily: (e.target as HTMLSelectElement).value as 'serif' | 'sans' }) },
          [m('option', { value: 'serif' }, 'Serif'), m('option', { value: 'sans' }, 'Sans-serif')]),
      ]),
      m('.form-check.form-switch', [
        m('input.form-check-input', { type: 'checkbox', role: 'switch', id: 'reduced-motion', checked: p.reducedMotion, onchange: (e: Event) => prefs.set({ reducedMotion: (e.target as HTMLInputElement).checked }) }),
        m('label.form-check-label', { for: 'reduced-motion' }, 'Reduce motion'),
      ]),
      m('.reader-text.border.rounded.p-3.mt-3', { 'aria-label': 'Preview' }, m('p.mb-0', 'The lamp guttered as she turned the page, and the words on it rearranged themselves into a name she had not spoken in years.')),
    ]));
  },
};

export function SettingsPage(): m.Component {
  return {
    oninit: () => setTitle('Settings'),
    view: () => {
      const u = session.user;
      return m('.page-text', [
        pageHeader('Settings'),
        m('ul.nav.nav-tabs.mb-4', [
          m('li.nav-item', m(m.route.Link, { href: '/settings', class: 'nav-link active', 'aria-current': 'page' }, 'Account')),
          m('li.nav-item', m(m.route.Link, { href: '/settings/patreon', class: 'nav-link' }, 'Patreon')),
        ]),
        u ? [m(ProfileCard, { user: u }), m(PasswordCard), m(SessionsCard, { user: u })] : null,
        m(PrefsCard),
      ]);
    },
  };
}

export function PatreonSettingsPage(): m.Component {
  return {
    oninit: () => setTitle('Patreon'),
    view: () =>
      m('.page-text', [
        pageHeader('Patreon early access'),
        m('ul.nav.nav-tabs.mb-4', [
          m('li.nav-item', m(m.route.Link, { href: '/settings', class: 'nav-link' }, 'Account')),
          m('li.nav-item', m(m.route.Link, { href: '/settings/patreon', class: 'nav-link active', 'aria-current': 'page' }, 'Patreon')),
        ]),
        m('p', 'All main fiction eventually releases free. Supporting us on Patreon provides early access while your membership remains entitled.'),
        emptyState('link-45deg', 'Patreon linking is not available yet', 'Connecting a Patreon account arrives with the Patreon integration. Every chapter still becomes free on its public date.'),
      ]),
  };
}

export function placeholderAccountPage(title: string, iconName: string, text: string): () => m.Component {
  return () => ({
    oninit: () => setTitle(title),
    view: () => m('.page-text', [pageHeader(title), emptyState(iconName, `Your ${title.toLowerCase()} is empty`, text)]),
  });
}
