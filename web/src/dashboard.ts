// Studio/admin SPA entrypoint, served for /studio and /admin paths.
import 'bootstrap/dist/css/bootstrap.min.css';
import 'bootstrap-icons/font/bootstrap-icons.css';
import './styles/app.css';
import m from 'mithril';
import { Shell } from './admin/shell';
import * as pages from './admin/pages';
import { config } from './stores/config';
import { prefs } from './stores/prefs';
import { session, type User } from './stores/session';
import { loginHref } from './utils/redirect';

type Gate = (u: User) => boolean;
const studio: Gate = (u) => u.permissions.studio;
const staff: Gate = (u) => u.permissions.admin;
const owner: Gate = (u) => u.role === 'owner';

function route(c: () => m.Component, allowed: Gate): m.RouteResolver {
  return {
    onmatch: async () => {
      await session.load();
      if (!session.user) {
        // Sign-in lives in the reader SPA: a full navigation is intended.
        window.location.assign(loginHref(window.location.pathname + window.location.search));
      }
      return undefined;
    },
    render: () => m(Shell, { allowed }, m(c, { key: m.route.get() })),
  };
}

const soon = (title: string) => pages.Placeholder(title, 'This screen arrives with the publishing studio.');

prefs.apply();
m.route.prefix = '';
void config.load();
m.route(document.getElementById('app')!, '/studio', {
  '/studio': route(soon('Overview'), studio),
  '/studio/works': route(soon('Works'), studio),
  '/studio/works/new': route(soon('New work'), studio),
  '/studio/works/:id/edit': route(soon('Edit work'), studio),
  '/studio/works/:id/entries': route(soon('Entries'), studio),
  '/studio/entries/:id/edit': route(soon('Edit entry'), studio),
  '/studio/entries/:id/schedule': route(soon('Schedule entry'), studio),
  '/studio/collections': route(soon('Collections'), studio),
  '/studio/authors': route(soon('Pen names'), studio),
  '/studio/analytics': route(soon('Analytics'), studio),
  '/studio/analytics/:workId': route(soon('Work analytics'), studio),
  '/studio/comments': route(soon('Comments'), studio),
  '/studio/reviews': route(soon('Reviews'), studio),
  '/studio/media': route(soon('Media'), studio),
  '/studio/integrations/patreon': route(soon('Patreon'), owner),
  '/studio/settings': route(soon('Settings'), studio),
  '/admin': route(pages.UsersPage, staff),
  '/admin/users': route(pages.UsersPage, staff),
  '/admin/reports': route(soon('Reports'), staff),
  '/admin/moderation': route(soon('Moderation'), staff),
  '/admin/discovery': route(soon('Discovery'), owner),
  '/admin/jobs': route(pages.Placeholder('Jobs', 'Job administration requires a separate operator (Collar superuser) session.'), owner),
  '/admin/audit': route(pages.AuditPage, owner),
  '/admin/system': route(soon('System'), owner),
  '/studio/:rest...': route(pages.Placeholder('Not found', 'No such studio page.'), studio),
  '/admin/:rest...': route(pages.Placeholder('Not found', 'No such admin page.'), staff),
});
