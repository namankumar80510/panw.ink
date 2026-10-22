// Public catalogue pages (§4.1): honest empty states, never sample stories or
// invented statistics.
import m from 'mithril';
import { reader, request } from '../api/reader';
import type { CollectionCard, Home, Ref, WorkCard } from '../api/types';
import { cardGrid, WorkCardView } from '../components/workcard';
import { alertBox, emptyState, icon, pageHeader, spinner } from '../components/ui';
import { load, loaded, type Loaded } from '../utils/load';
import { setTitle } from '../utils/title';

function errorOr<T>(l: Loaded<T>, render: (d: T) => m.Children): m.Children {
  if (l.loading && !l.data) return spinner();
  if (l.error) {
    return l.error.status === 404
      ? emptyState('question-circle', 'Not found', ['This page does not exist or is no longer available. ', m(m.route.Link, { href: '/' }, 'Return home'), '.'])
      : alertBox('danger', l.error.message);
  }
  return l.data ? render(l.data) : null;
}

function rail(id: string, title: string, works: WorkCard[], more?: [string, string]): m.Children {
  if (!works.length) return null;
  return m('section.mb-5', { 'aria-labelledby': id }, [
    m('.d-flex.justify-content-between.align-items-baseline.mb-3', [
      m('h2.h4.rail-title.mb-0', { id }, title),
      more ? m(m.route.Link, { href: more[0], class: 'small' }, more[1]) : null,
    ]),
    cardGrid(works),
  ]);
}

export function HomePage(): m.Component {
  const h = loaded<Home>();
  return {
    oninit: () => { setTitle(); void load(h, () => reader.get<Home>('/home')); },
    view: () => [
      m('section.py-4.mb-4.border-bottom', [
        m('h1.display-6.rail-title', 'Original fiction, free to read.'),
        m('p.lead.text-body-secondary.page-text', 'Complete short stories and serialized novels. No account needed to read. Patreon supporters may read some chapters early — everything becomes free on its public date.'),
        m('.d-flex.flex-wrap.gap-2', [
          m(m.route.Link, { href: '/stories', class: 'btn btn-primary' }, 'Browse short stories'),
          m(m.route.Link, { href: '/novels', class: 'btn btn-outline-primary' }, 'Browse novels'),
        ]),
      ]),
      errorOr(h, (d) => {
        const empty = !d.featured.length && !d.short_reads.length && !d.recently_updated.length;
        if (empty) return emptyState('book', 'Nothing published yet', 'Stories and novels appear here as soon as they are released.');
        return [
          rail('featured', 'Featured', d.featured),
          // §4.5: at least one completed/short-story rail on home.
          rail('short-reads', 'Complete short stories', d.short_reads, ['/stories', 'All short stories']),
          rail('ongoing', 'Ongoing novels', d.ongoing_novels, ['/novels', 'All novels']),
          rail('completed', 'Completed', d.completed),
          rail('updated', 'Recently updated', d.recently_updated),
        ];
      }),
    ],
  };
}

interface Genre { slug: string; name: string }
let genresCache: Genre[] | null = null;
async function genres(): Promise<Genre[]> {
  genresCache ??= await reader.get<Genre[]>('/genres');
  return genresCache;
}

/** Shared catalogue page for /novels, /stories and parts of /discover. */
function Catalogue(kind: 'novel' | 'short_story', title: string, lead: string): () => m.Component {
  return () => {
    const s = {
      items: [] as WorkCard[], next: null as string | null, loading: false, error: '',
      genre: '', status: '', sort: 'latest', length: '', genreList: [] as Genre[],
    };
    const params = () => {
      const p = new URLSearchParams({ kind, sort: s.sort, limit: '24' });
      if (s.genre) p.set('genre', s.genre);
      if (s.status) p.set('status', s.status);
      if (s.length === 'short') p.set('max_minutes', '20');
      if (s.length === 'medium') { p.set('min_words', String(20 * 230)); p.set('max_minutes', '90'); }
      if (s.length === 'long') p.set('min_words', String(90 * 230));
      return p;
    };
    const fetchPage = async (reset: boolean) => {
      s.loading = true; s.error = '';
      try {
        const p = params();
        if (!reset && s.next) p.set('cursor', s.next);
        const env = await request<WorkCard[]>('/works?' + p.toString());
        s.items = reset ? env.data : s.items.concat(env.data);
        s.next = env.meta.next_cursor;
      } catch (e) {
        s.error = (e as Error).message;
      } finally {
        s.loading = false; m.redraw();
      }
    };
    const filter = (label: string, id: string, value: string, options: Array<[string, string]>, set: (v: string) => void) =>
      m('.col-6.col-md-3', [
        m('label.form-label.small', { for: id }, label),
        m('select.form-select.form-select-sm', { id, value, onchange: (e: Event) => { set((e.target as HTMLSelectElement).value); void fetchPage(true); } },
          options.map(([v, l]) => m('option', { value: v, selected: v === value }, l))),
      ]);
    return {
      oninit: async () => {
        setTitle(title);
        s.genre = m.route.param('genre') ?? '';
        void fetchPage(true);
        try { s.genreList = await genres(); m.redraw(); } catch { /* filters still work without names */ }
      },
      view: () => [
        pageHeader(title, lead),
        m('form.row.g-2.mb-4', { role: 'search', 'aria-label': 'Filters', onsubmit: (e: Event) => e.preventDefault() }, [
          filter('Genre', 'f-genre', s.genre, [['', 'All genres'], ...s.genreList.map((g): [string, string] => [g.slug, g.name])], (v) => (s.genre = v)),
          kind === 'novel' ? filter('Status', 'f-status', s.status, [['', 'Any status'], ['ongoing', 'Ongoing'], ['completed', 'Complete'], ['hiatus', 'On hiatus']], (v) => (s.status = v)) : null,
          filter('Length', 'f-length', s.length, [['', 'Any length'], ['short', 'Under 20 min'], ['medium', '20–90 min'], ['long', 'Over 90 min']], (v) => (s.length = v)),
          filter('Sort', 'f-sort', s.sort, [['latest', 'Recently updated'], ['newest', 'Newest'], ['title', 'Title A–Z']], (v) => (s.sort = v)),
        ]),
        s.error ? alertBox('danger', s.error) : null,
        s.items.length ? cardGrid(s.items) : s.loading ? spinner() : emptyState(kind === 'novel' ? 'book' : 'file-earmark-text', 'Nothing matches', 'Try different filters, or check back soon.'),
        s.next ? m('.text-center.mt-4', m('button.btn.btn-outline-primary', { type: 'button', disabled: s.loading, onclick: () => fetchPage(false) }, s.loading ? 'Loading…' : 'Load more')) : null,
      ],
    };
  };
}

export const NovelsPage = Catalogue('novel', 'Novels', 'Serialized fiction, chapter by chapter.');
export const StoriesPage = Catalogue('short_story', 'Short stories', 'Complete stories you can finish in one sitting.');

export function DiscoverPage(): m.Component {
  const s = { latest: loaded<WorkCard[]>(), stories: loaded<WorkCard[]>(), novels: loaded<WorkCard[]>() };
  return {
    oninit: () => {
      setTitle('Discover');
      void load(s.latest, () => reader.get<WorkCard[]>('/works?sort=latest&limit=8'));
      void load(s.stories, () => reader.get<WorkCard[]>('/works?kind=short_story&sort=newest&limit=8'));
      void load(s.novels, () => reader.get<WorkCard[]>('/works?kind=novel&sort=newest&limit=8'));
    },
    view: () => [
      pageHeader('Discover', 'Rankings by real reader satisfaction (Trending, Top rated, New & underexplored) appear once there are enough readers to rank fairly. Until then, here is what is new.'),
      errorOr(s.latest, (d) => rail('d-latest', 'Latest updates', d)),
      errorOr(s.stories, (d) => rail('d-stories', 'New short stories', d, ['/stories', 'All short stories'])),
      errorOr(s.novels, (d) => rail('d-novels', 'New novels', d, ['/novels', 'All novels'])),
    ],
  };
}

export function CollectionsPage(): m.Component {
  const l = loaded<CollectionCard[]>();
  return {
    oninit: () => { setTitle('Collections'); void load(l, () => reader.get<CollectionCard[]>('/collections')); },
    view: () => [
      pageHeader('Collections', 'Curated anthologies of independent short stories.'),
      errorOr(l, (d) => d.length
        ? m('.row.row-cols-1.row-cols-md-2.g-3', d.map((c) => m('.col', { key: c.slug }, m('article.card.h-100', m('.card-body', [
            m('h2.h5.rail-title', m(m.route.Link, { href: '/collections/' + c.slug, class: 'stretched-link text-decoration-none' }, c.title)),
            m('p.small.text-body-secondary.mb-2', [`${c.story_count} ${c.story_count === 1 ? 'story' : 'stories'} · by `, c.author.name]),
            m('p.mb-0', c.description),
          ])))))
        : emptyState('collection', 'No collections yet', 'Anthologies will appear here once curated.')),
    ],
  };
}

export function CollectionPage(): m.Component {
  const l = loaded<CollectionCard & { stories: WorkCard[] }>();
  return {
    oninit: () => { void load(l, () => reader.get('/collections/' + encodeURIComponent(m.route.param('slug')))).then(() => setTitle(l.data?.title ?? 'Collection')); },
    view: () => errorOr(l, (c) => [
      pageHeader(c.title, [c.description, m('br'), m('small', ['Curated by ', m(m.route.Link, { href: '/authors/' + c.author.slug }, c.author.name)])]),
      c.stories.length
        ? m('ol.list-unstyled.row.row-cols-1.row-cols-md-2.row-cols-xl-3.g-3', c.stories.map((w, i) => m('li.col', { key: w.id }, [
            m('.small.text-body-secondary.mb-1', `Story ${i + 1}`),
            m(WorkCardView, { work: w }),
          ])))
        : emptyState('collection', 'No stories are public yet', 'Stories in this collection appear here once they are released.'),
    ]),
  };
}

export function AuthorPage(): m.Component {
  const l = loaded<{ slug: string; name: string; bio: string; website_url: string | null; works: WorkCard[] }>();
  return {
    oninit: () => { void load(l, () => reader.get('/authors/' + encodeURIComponent(m.route.param('slug')))).then(() => setTitle(l.data?.name ?? 'Author')); },
    view: () => errorOr(l, (a) => {
      const completed = a.works.filter((w) => w.publication_status === 'completed');
      const ongoing = a.works.filter((w) => w.publication_status !== 'completed');
      return [
        pageHeader(a.name, a.bio),
        a.website_url ? m('p', m('a', { href: a.website_url, rel: 'noopener nofollow', target: '_blank' }, [icon('box-arrow-up-right'), ' Website'])) : null,
        // Following pen names arrives with the reader library.
        a.works.length ? null : emptyState('journal', 'No public works yet', ''),
        rail('a-ongoing', 'Ongoing', ongoing),
        rail('a-completed', 'Complete', completed),
      ];
    }),
  };
}

export function SearchPage(): m.Component {
  const s = { q: '', ran: '', res: loaded<{ works: WorkCard[]; authors: Ref[]; collections: Ref[] }>() };
  const run = () => {
    s.q = s.q.trim();
    if (!s.q) return;
    s.ran = s.q;
    m.route.set('/search', { q: s.q }, { replace: true });
    void load(s.res, () => reader.get('/search?q=' + encodeURIComponent(s.q)));
  };
  return {
    oninit: () => { setTitle('Search'); s.q = m.route.param('q') ?? ''; s.res.loading = false; if (s.q) run(); },
    view: () => [
      pageHeader('Search', 'Find works, pen names and collections.'),
      m('form.d-flex.gap-2.mb-4', { role: 'search', onsubmit: (e: Event) => { e.preventDefault(); run(); } }, [
        m('label.visually-hidden', { for: 'q' }, 'Search'),
        m('input.form-control', { id: 'q', type: 'search', value: s.q, maxlength: 100, placeholder: 'Title, synopsis, pen name…', oninput: (e: Event) => (s.q = (e.target as HTMLInputElement).value) }),
        m('button.btn.btn-primary', { type: 'submit' }, [icon('search'), m('span.visually-hidden', 'Search')]),
      ]),
      s.ran ? errorOr(s.res, (r) => {
        if (!r.works.length && !r.authors.length && !r.collections.length) return emptyState('search', `No results for “${s.ran}”`, 'Try a shorter or different search.');
        return [
          r.authors.length ? m('section.mb-4', [m('h2.h5', 'Pen names'), m('ul', r.authors.map((a) => m('li', m(m.route.Link, { href: '/authors/' + a.slug }, a.name))))]) : null,
          r.collections.length ? m('section.mb-4', [m('h2.h5', 'Collections'), m('ul', r.collections.map((c) => m('li', m(m.route.Link, { href: '/collections/' + c.slug }, c.name))))]) : null,
          r.works.length ? m('section', [m('h2.h5', 'Works'), cardGrid(r.works)]) : null,
        ];
      }) : null,
    ],
  };
}

export function NotFoundPage(): m.Component {
  return {
    oninit: () => setTitle('Not found'),
    view: () => [pageHeader('Page not found'), m('p', ['We could not find that page. ', m(m.route.Link, { href: '/' }, 'Return home'), '.'])],
  };
}
