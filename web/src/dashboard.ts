// Studio/admin SPA entrypoint, served for /studio and /admin paths.
import 'bootstrap/dist/css/bootstrap.min.css';
import 'bootstrap-icons/font/bootstrap-icons.css';
import './styles/app.css';
import m from 'mithril';
import { Shell } from './admin/shell';
import * as pages from './admin/pages';
import * as studio from './admin/studio';
import { Editor } from './admin/editor';
import { Schedule } from './admin/schedule';
import { config } from './stores/config';
import { prefs } from './stores/prefs';
import { session, type User } from './stores/session';
import { loginHref } from './utils/redirect';

type Gate = (u: User) => boolean;
const canStudio: Gate = (u) => u.permissions.studio;
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
    render: () => m(Shell, { allowed }, [m(c, { key: m.route.get() }), m(studio.CompleteConfirm, { key: 'complete-confirm' })]),
  };
}

const soon = (title: string) => pages.Placeholder(title, 'This screen arrives with the publishing studio.');

prefs.apply();
m.route.prefix = '';
void config.load();
m.route(document.getElementById('app')!, '/studio', {
  '/studio': route(studio.Overview, canStudio),
  '/studio/works': route(studio.WorksList, canStudio),
  '/studio/works/new': route(studio.NewWork, canStudio),
  '/studio/works/:id/edit': route(studio.WorkEdit, canStudio),
  '/studio/works/:id/entries': route(studio.Entries, canStudio),
  '/studio/entries/:id/edit': route(Editor, canStudio),
  '/studio/entries/:id/schedule': route(Schedule, canStudio),
  '/studio/collections': route(studio.Collections, canStudio),
  '/studio/authors': route(studio.PenNames, canStudio),
  '/studio/media': route(studio.Media, canStudio),
  '/studio/analytics': route(soon('Analytics'), canStudio),
  '/studio/analytics/:workId': route(soon('Work analytics'), canStudio),
  '/studio/comments': route(soon('Comments'), canStudio),
  '/studio/reviews': route(soon('Reviews'), canStudio),
  '/studio/integrations/patreon': route(soon('Patreon'), owner),
  '/studio/settings': route(soon('Settings'), canStudio),
  '/admin': route(pages.UsersPage, staff),
  '/admin/users': route(pages.UsersPage, staff),
  '/admin/reports': route(soon('Reports'), staff),
  '/admin/moderation': route(soon('Moderation'), staff),
  '/admin/discovery': route(soon('Discovery'), owner),
  '/admin/jobs': route(pages.Placeholder('Jobs', 'Job administration requires a separate operator (Collar superuser) session.'), owner),
  '/admin/audit': route(pages.AuditPage, owner),
  '/admin/system': route(soon('System'), owner),
  '/studio/:rest...': route(pages.Placeholder('Not found', 'No such studio page.'), canStudio),
  '/admin/:rest...': route(pages.Placeholder('Not found', 'No such admin page.'), staff),
});
