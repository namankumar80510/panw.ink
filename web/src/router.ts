// Reader SPA routes (§4.1, §4.2): plain history routing, no hash URLs. The
// Go server returns index.html for these paths, so reload/back/forward work.
import m from 'mithril';
import { Layout } from './components/layout';
import { session } from './stores/session';
import { loginHref } from './utils/redirect';
import * as pub from './pages/public';
import * as auth from './pages/auth';
import * as account from './pages/account';
import * as stat from './pages/static';

type Factory = () => m.Component;

let popped = false;
let lastRoute: string | null = null;
window.addEventListener('popstate', () => { popped = true; });

/**
 * After a navigation (not after ordinary redraws): top of page for new
 * routes, keep the browser's position on back/forward, move focus to main.
 */
function afterRoute(): void {
  const current = m.route.get();
  if (current === lastRoute) return;
  lastRoute = current;
  const wasPop = popped;
  popped = false;
  requestAnimationFrame(() => {
    if (!wasPop) window.scrollTo(0, 0);
    const main = document.getElementById('main');
    if (main && !main.contains(document.activeElement)) main.focus({ preventScroll: true });
  });
}

function page(c: Factory, opts: { auth?: boolean } = {}): m.RouteResolver {
  return {
    onmatch: async () => {
      if (opts.auth) {
        await session.load();
        if (!session.user) {
          m.route.set(loginHref(window.location.pathname + window.location.search), {}, { replace: true });
          return undefined;
        }
      }
      return undefined;
    },
    render: (vnode) => {
      afterRoute();
      return m(Layout, m(c, { ...vnode.attrs, key: m.route.get() }));
    },
  };
}

const detail = (label: string) => page(pub.DetailPlaceholder(label));

export const routes: Record<string, m.RouteResolver> = {
  '/': page(pub.HomePage),
  '/discover': page(pub.DiscoverPage),
  '/novels': page(pub.NovelsPage),
  '/stories': page(pub.StoriesPage),
  '/collections': page(pub.CollectionsPage),
  '/collections/:slug': detail('Collection'),
  '/authors/:slug': detail('Pen name'),
  '/fiction/:workSlug': detail('Work'),
  '/fiction/:workSlug/ch/:entrySlug': detail('Chapter'),
  '/stories/:workSlug/read': detail('Story'),
  '/reviews/:workSlug': detail('Reviews'),
  '/search': page(pub.SearchPage),
  '/support': page(stat.SupportPage),
  '/about': page(stat.AboutPage),
  '/policies/:kind': page(stat.PolicyPage),

  '/login': page(auth.LoginPage),
  '/register': page(auth.RegisterPage),
  '/verify-email': page(auth.VerifyEmailPage),
  '/forgot-password': page(auth.ForgotPasswordPage),
  '/reset-password': page(auth.ResetPasswordPage),

  '/library': page(account.placeholderAccountPage('Library', 'bookmark', 'Saved and followed works will appear here.'), { auth: true }),
  '/history': page(account.placeholderAccountPage('History', 'clock-history', 'Recently read chapters and stories will appear here.'), { auth: true }),
  '/notifications': page(account.placeholderAccountPage('Notifications', 'bell', 'Release and reply notices will appear here.'), { auth: true }),
  '/settings': page(account.SettingsPage, { auth: true }),
  '/settings/patreon': page(account.PatreonSettingsPage, { auth: true }),

  '/:404...': page(pub.NotFoundPage),
};
