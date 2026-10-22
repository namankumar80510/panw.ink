// Public catalogue routes (§4.1). Until the catalogue API lands (Phase 2)
// they render honest empty states — never sample stories or invented stats.
import m from 'mithril';
import { emptyState, pageHeader } from '../components/ui';
import { setTitle } from '../utils/title';

function simple(title: string, lead: string, iconName: string, emptyTitle: string, emptyBody: m.Children): () => m.Component {
  return () => ({
    oninit: () => setTitle(title),
    view: () => [pageHeader(title, lead), emptyState(iconName, emptyTitle, emptyBody)],
  });
}

export function HomePage(): m.Component {
  return {
    oninit: () => setTitle(),
    view: () => [
      m('section.py-4.mb-4.border-bottom', [
        m('h1.display-6.rail-title', 'Original fiction, free to read.'),
        m('p.lead.text-body-secondary.page-text', 'Complete short stories and serialized novels. No account needed to read. Patreon supporters may read some chapters early — everything becomes free on its public date.'),
        m('.d-flex.flex-wrap.gap-2', [
          m(m.route.Link, { href: '/stories', class: 'btn btn-primary' }, 'Browse short stories'),
          m(m.route.Link, { href: '/novels', class: 'btn btn-outline-primary' }, 'Browse novels'),
        ]),
      ]),
      m('section.mb-5', { 'aria-labelledby': 'short-reads' }, [
        m('h2.h4.rail-title', { id: 'short-reads' }, 'Complete short stories'),
        emptyState('file-earmark-text', 'Nothing published yet', 'Finished short stories will appear here as soon as they are released.'),
      ]),
      m('section.mb-5', { 'aria-labelledby': 'ongoing' }, [
        m('h2.h4.rail-title', { id: 'ongoing' }, 'Novels'),
        emptyState('book', 'No novels yet', 'Serialized novels will appear here when their first chapter is public.'),
      ]),
    ],
  };
}

export const DiscoverPage = simple('Discover', 'Trending, top rated, new and underexplored — ranked by real reader satisfaction, separately for short stories and novels.', 'compass', 'Not enough readers yet', 'Discovery lists appear once there is enough reading activity to rank fairly. Until then, browse the catalogue directly.');
export const NovelsPage = simple('Novels', 'Serialized fiction, chapter by chapter.', 'book', 'No novels published yet', 'Check back soon.');
export const StoriesPage = simple('Short stories', 'Complete stories you can finish in one sitting.', 'file-earmark-text', 'No short stories published yet', 'Check back soon.');
export const CollectionsPage = simple('Collections', 'Curated anthologies of independent short stories.', 'collection', 'No collections yet', 'Anthologies will appear here once curated.');
export const SearchPage = simple('Search', 'Find works, pen names and collections.', 'search', 'Search is not available yet', 'Search arrives with the catalogue.');

export function NotFoundPage(): m.Component {
  return {
    oninit: () => setTitle('Not found'),
    view: () => [pageHeader('Page not found'), m('p', ['We could not find that page. ', m(m.route.Link, { href: '/' }, 'Return home'), '.'])],
  };
}

/** Detail routes render a not-available state until their API exists. */
export function DetailPlaceholder(kind: string): () => m.Component {
  return () => ({
    oninit: () => setTitle(kind),
    view: () => [pageHeader(kind), emptyState('question-circle', 'Not available', 'This page will show published content once the catalogue is live.')],
  });
}
