// Studio/admin shell (§4.3). Navigation is hidden from people without the
// role; every API independently enforces permissions.
import m from 'mithril';
import Offcanvas from 'bootstrap/js/dist/offcanvas';
import { Toasts } from '../components/toasts';
import { icon } from '../components/ui';
import { session, type User } from '../stores/session';

export interface NavItem {
  href: string;
  label: string;
  icon: string;
  show: (u: User) => boolean;
}

const studio = (u: User) => u.permissions.studio;
const staff = (u: User) => u.permissions.admin;
const owner = (u: User) => u.role === 'owner';

export const NAV: Array<{ title: string; items: NavItem[] }> = [
  {
    title: 'Studio',
    items: [
      { href: '/studio', label: 'Overview', icon: 'speedometer2', show: studio },
      { href: '/studio/works', label: 'Works', icon: 'journal-text', show: studio },
      { href: '/studio/collections', label: 'Collections', icon: 'collection', show: studio },
      { href: '/studio/authors', label: 'Pen names', icon: 'person-badge', show: studio },
      { href: '/studio/analytics', label: 'Analytics', icon: 'graph-up', show: studio },
      { href: '/studio/comments', label: 'Comments', icon: 'chat-left-text', show: studio },
      { href: '/studio/reviews', label: 'Reviews', icon: 'star', show: studio },
      { href: '/studio/media', label: 'Media', icon: 'images', show: studio },
      { href: '/studio/integrations/patreon', label: 'Patreon', icon: 'link-45deg', show: owner },
      { href: '/studio/settings', label: 'Settings', icon: 'gear', show: studio },
    ],
  },
  {
    title: 'Admin',
    items: [
      { href: '/admin/users', label: 'Users', icon: 'people', show: staff },
      { href: '/admin/reports', label: 'Reports', icon: 'flag', show: staff },
      { href: '/admin/moderation', label: 'Moderation', icon: 'shield-check', show: staff },
      { href: '/admin/discovery', label: 'Discovery', icon: 'compass', show: owner },
      { href: '/admin/jobs', label: 'Jobs', icon: 'list-task', show: owner },
      { href: '/admin/audit', label: 'Audit log', icon: 'journal-check', show: owner },
      { href: '/admin/system', label: 'System', icon: 'hdd-network', show: owner },
    ],
  },
];

let menu: Offcanvas | null = null;

function sidebar(u: User): m.Children {
  const current = m.route.get() ?? '';
  return NAV.map((section) => {
    const items = section.items.filter((i) => i.show(u));
    if (!items.length) return null;
    return [
      m('h2.h6.text-uppercase.text-body-secondary.mt-3.mb-2.small', section.title),
      m('ul.nav.flex-column', items.map((i) => {
        const active = current === i.href || (i.href !== '/studio' && current.startsWith(i.href + '/'));
        return m('li.nav-item', m(m.route.Link, {
          href: i.href, class: 'nav-link px-0' + (active ? ' active' : ''), 'aria-current': active ? 'page' : undefined,
          onclick: () => menu?.hide(),
        }, [icon(i.icon), ' ', i.label]));
      })),
    ];
  });
}

export const Shell: m.Component<{ allowed: (u: User) => boolean }> = {
  view: ({ attrs, children }) => {
    const u = session.user;
    if (!session.ready) return m('.container.py-5', m('.spinner-border', { role: 'status' }, m('span.visually-hidden', 'Loading')));
    if (!u) return null; // redirected to sign-in by the route resolver
    return [
      m('a.skip-link', { href: '#main' }, 'Skip to content'),
      m('nav.navbar.border-bottom.bg-body-tertiary', { 'aria-label': 'Studio' }, m('.container-fluid', [
        m('button.btn.btn-outline-secondary.d-lg-none.me-2', { type: 'button', 'aria-label': 'Open navigation', onclick: () => menu?.toggle() }, icon('list')),
        m('a.navbar-brand', { href: '/' }, [icon('feather'), ' Pawn.Ink ', m('span.badge.text-bg-secondary.align-middle', 'Studio')]),
        m('span.navbar-text.small.ms-auto', [u.display_name, ' · ', u.role]),
      ])),
      m('.container-fluid', m('.row', [
        m('aside.col-lg-auto.border-end', m('.offcanvas-lg.offcanvas-start.dash-sidebar', {
          id: 'dashNav', tabindex: -1,
          oncreate: (v: m.VnodeDOM) => { menu = Offcanvas.getOrCreateInstance(v.dom as HTMLElement); },
          onremove: () => { menu?.dispose(); menu = null; },
        }, [
          m('.offcanvas-header', [m('h2.offcanvas-title.h5', 'Navigation'), m('button.btn-close', { type: 'button', 'aria-label': 'Close', onclick: () => menu?.hide() })]),
          m('.offcanvas-body.flex-column.py-2', sidebar(u)),
        ])),
        m('main#main.col.py-4', { tabindex: -1 },
          attrs.allowed(u)
            ? children
            : m('.alert.alert-warning', { role: 'alert' }, ['You do not have access to this area. ', m('a', { href: '/' }, 'Return to the library'), '.'])),
      ])),
      m(Toasts),
    ];
  },
};
