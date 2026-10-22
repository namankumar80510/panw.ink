// Reader layout: skip link, header with responsive offcanvas navigation,
// content, footer and toasts (§4.4 Header).
import m from 'mithril';
import Offcanvas from 'bootstrap/js/dist/offcanvas';
import { config } from '../stores/config';
import { session } from '../stores/session';
import { loginHref } from '../utils/redirect';
import { icon } from './ui';
import { Toasts, toast } from './toasts';

const NAV: Array<[string, string, string]> = [
  ['/discover', 'Discover', 'compass'],
  ['/stories', 'Short stories', 'file-earmark-text'],
  ['/novels', 'Novels', 'book'],
  ['/collections', 'Collections', 'collection'],
];

function isActive(href: string): boolean {
  const p = m.route.get() ?? '';
  return p === href || p.startsWith(href + '/') || p.startsWith(href + '?');
}

function navLink(href: string, label: string, iconName: string, close: () => void): m.Vnode {
  const active = isActive(href);
  return m('li.nav-item',
    m(m.route.Link, { href, class: 'nav-link' + (active ? ' active' : ''), 'aria-current': active ? 'page' : undefined, onclick: close },
      [icon(iconName), ' ', label]),
  );
}

let offcanvas: Offcanvas | null = null;
const closeMenu = () => offcanvas?.hide();

async function signOut(): Promise<void> {
  closeMenu();
  try {
    await session.logout();
    toast('You are signed out.', 'success');
  } catch {
    toast('Sign-out could not reach the server; your browser session was cleared.', 'info');
  }
  m.route.set('/');
}

const Header: m.Component = {
  view: () => {
    const u = session.user;
    return m('header', [
      m('a.skip-link', { href: '#main' }, 'Skip to content'),
      config.value?.non_production
        ? m('.nonprod-banner', { role: 'note' }, `Non-production environment (${config.value.environment}). Data here is not real.`)
        : null,
      m('nav.navbar.navbar-expand-lg.border-bottom.bg-body-tertiary', { 'aria-label': 'Main' },
        m('.container', [
          m(m.route.Link, { href: '/', class: 'navbar-brand' }, [icon('feather'), ' ', config.appName]),
          m('button.navbar-toggler', {
            type: 'button', 'aria-controls': 'mainMenu', 'aria-label': 'Open menu',
            onclick: () => offcanvas?.toggle(),
          }, m('span.navbar-toggler-icon')),
          m('.offcanvas.offcanvas-end', {
            id: 'mainMenu', tabindex: -1, 'aria-labelledby': 'mainMenuLabel',
            oncreate: (v: m.VnodeDOM) => { offcanvas = Offcanvas.getOrCreateInstance(v.dom as HTMLElement); },
            onremove: () => { offcanvas?.dispose(); offcanvas = null; },
          }, [
            m('.offcanvas-header', [
              m('h2.offcanvas-title.h5', { id: 'mainMenuLabel' }, 'Menu'),
              m('button.btn-close', { type: 'button', 'aria-label': 'Close menu', onclick: closeMenu }),
            ]),
            m('.offcanvas-body', [
              m('ul.navbar-nav.me-auto', NAV.map(([h, l, i]) => navLink(h, l, i, closeMenu))),
              m('ul.navbar-nav', [
                navLink('/search', 'Search', 'search', closeMenu),
                ...(u
                  ? [
                      navLink('/library', 'Library', 'bookmark', closeMenu),
                      u.permissions.studio || u.permissions.admin
                        ? m('li.nav-item', m('a.nav-link', { href: u.permissions.studio ? '/studio' : '/admin' }, [icon('pencil-square'), ' Studio']))
                        : null,
                      navLink('/settings', u.display_name, 'person-circle', closeMenu),
                      m('li.nav-item', m('button.nav-link.btn.btn-link', { type: 'button', onclick: signOut }, [icon('box-arrow-right'), ' Sign out'])),
                    ]
                  : [
                      m('li.nav-item', m(m.route.Link, { href: loginHref(), class: 'nav-link', onclick: closeMenu }, [icon('box-arrow-in-right'), ' Sign in'])),
                      m('li.nav-item', m(m.route.Link, { href: '/register', class: 'btn btn-outline-primary btn-sm ms-lg-2 mt-2 mt-lg-0', onclick: closeMenu }, 'Create account')),
                    ]),
              ]),
            ]),
          ]),
        ]),
      ),
    ]);
  },
};

const Footer: m.Component = {
  view: () =>
    m('footer.border-top.py-4.mt-5.small.text-body-secondary',
      m('.container.d-flex.flex-wrap.gap-3.justify-content-between', [
        m('span', `${config.appName} — original fiction, free to read. Supporters may read some chapters early; everything releases free.`),
        m('nav', { 'aria-label': 'Footer' }, m('ul.list-inline.mb-0', [
          ['/about', 'About'], ['/support', 'Support'], ['/policies/terms', 'Terms'], ['/policies/privacy', 'Privacy'],
          ['/policies/content', 'Content policy'], ['/policies/community', 'Community'], ['/policies/copyright', 'Copyright'],
        ].map(([h, l]) => m('li.list-inline-item', m(m.route.Link, { href: h!, class: 'link-secondary' }, l))))),
      ]),
    ),
};

/** Layout wraps a page. `wide` pages use the full container. */
export const Layout: m.Component<{ wide?: boolean }> = {
  view: ({ children, attrs }) => [
    m(Header),
    m('main#main.py-4', { tabindex: -1 }, m(attrs.wide === false ? '.container.page-text' : '.container', children)),
    m(Footer),
    m(Toasts),
  ],
};
