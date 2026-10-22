// Studio/admin screens. Admin listings read native Collar collections
// directly (§7.5) through the separate native client; studio screens arrive
// with the publishing studio (Phase 3).
import m from 'mithril';
import { collar, type CollarList } from '../api/collar';
import { alertBox, emptyState, errorText, pageHeader, spinner } from '../components/ui';
import { setTitle } from '../utils/title';

export function Placeholder(title: string, text: string): () => m.Component {
  return () => ({
    oninit: () => setTitle(title),
    view: () => [pageHeader(title), emptyState('tools', 'Not built yet', text)],
  });
}

interface UserSummary {
  id: string;
  email: string;
  handle: string | null;
  display_name: string;
  role: string;
  state: string;
  email_verified_at: string | null;
  created_at: string;
}

interface AuditRow {
  id: string;
  actor_user_id: string | null;
  action: string;
  object_type: string;
  object_id: string | null;
  request_id: string;
  created_at: string;
}

function nativeTable<T>(title: string, table: string, sort: string, cols: Array<[string, (r: T) => m.Children]>): () => m.Component {
  return () => {
    let page = 1;
    let data: CollarList<T> | null = null;
    let error = '';
    const load = async () => {
      data = null; error = '';
      try { data = await collar.list<T>(table, { sort, page, perPage: 25 }); } catch (e) { error = errorText(e); }
      m.redraw();
    };
    return {
      oninit: () => { setTitle(title); void load(); },
      view: () => [
        pageHeader(title),
        error ? alertBox('danger', error) : data === null ? spinner() : data.items.length === 0 ? emptyState('inbox', 'Nothing here yet', '') : [
          m('.table-responsive', m('table.table.table-sm.align-middle', [
            m('thead', m('tr', cols.map(([h]) => m('th', { scope: 'col' }, h)))),
            m('tbody', data.items.map((r) => m('tr', cols.map(([, f]) => m('td', f(r)))))),
          ])),
          m('nav', { 'aria-label': 'Pagination' }, m('ul.pagination', [
            m('li.page-item', { class: page <= 1 ? 'disabled' : '' }, m('button.page-link', { type: 'button', disabled: page <= 1, onclick: () => { page--; void load(); } }, 'Previous')),
            m('li.page-item.active', m('span.page-link', { 'aria-current': 'page' }, String(page))),
            m('li.page-item', { class: (data.totalPages ?? page) <= page ? 'disabled' : '' }, m('button.page-link', { type: 'button', disabled: (data.totalPages ?? page) <= page, onclick: () => { page++; void load(); } }, 'Next')),
          ])),
        ],
      ],
    };
  };
}

const when = (ts: string | null) => (ts ? new Date(ts).toLocaleString() : '—');

export const UsersPage = nativeTable<UserSummary>('Users', 'admin_user_summaries', '-created_at', [
  ['ID', (r) => r.id],
  ['Name', (r) => [r.display_name, r.handle ? m('div.small.text-body-secondary', '@' + r.handle) : null]],
  ['Email', (r) => r.email],
  ['Role', (r) => m('span.badge.text-bg-light.border', r.role)],
  ['State', (r) => r.state],
  ['Verified', (r) => (r.email_verified_at ? 'yes' : 'no')],
  ['Joined', (r) => when(r.created_at)],
]);

export const AuditPage = nativeTable<AuditRow>('Audit log', 'audit_logs', '-created_at', [
  ['When', (r) => when(r.created_at)],
  ['Action', (r) => m('code', r.action)],
  ['Object', (r) => `${r.object_type}${r.object_id ? ' #' + r.object_id : ''}`],
  ['Actor', (r) => r.actor_user_id ?? 'system/CLI'],
  ['Request', (r) => m('small.text-body-secondary', r.request_id)],
]);
