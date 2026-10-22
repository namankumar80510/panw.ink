// Work overview (§4.1 /fiction/:workSlug): metadata, synopsis, tags,
// warnings, author, release states and the start/next action. A short story
// is presented as one complete text: "Read story", never "Chapter 1".
import m from 'mithril';
import { reader } from '../api/reader';
import type { TOCEntry, WorkDetail } from '../api/types';
import { alertBox, emptyState, icon, spinner } from '../components/ui';
import { entryHref, formatCount, formatDate, minutes, STATUS_LABEL } from '../utils/format';
import { load, loaded } from '../utils/load';
import { setTitle } from '../utils/title';

export function accessBadge(e: { access: string; public_at: string | null }): m.Children {
  if (e.access === 'public') return null;
  if (e.access === 'early') return m('span.badge.text-bg-info.ms-2', { title: 'Supporters can read this now' }, ['Early access · free ', formatDate(e.public_at)]);
  return m('span.badge.text-bg-light.border.ms-2', ['Free ', formatDate(e.public_at)]);
}

function tocList(entries: TOCEntry[], workSlug: string): m.Children {
  let lastVolume = -1;
  const items: m.Children[] = [];
  for (const e of entries) {
    if (e.volume && e.volume.number !== lastVolume) {
      lastVolume = e.volume.number;
      items.push(m('li.list-group-item.bg-body-tertiary.fw-semibold.small', { key: 'v' + e.volume.number }, `Volume ${e.volume.number}: ${e.volume.title}`));
    }
    items.push(m('li.list-group-item.d-flex.justify-content-between.align-items-center', { key: e.id }, [
      m('span', [
        m('span.text-body-secondary.me-2', `${e.ordinal}.`),
        e.access === 'upcoming' ? m('span', e.title) : m(m.route.Link, { href: entryHref('novel', workSlug, e.slug) }, e.title),
        accessBadge(e),
      ]),
      m('small.text-body-secondary.text-nowrap.ms-2', minutes(e.read_seconds)),
    ]));
  }
  return m('ol.list-group.list-unstyled', items);
}

export function WorkPage(): m.Component {
  const w = loaded<WorkDetail>();
  const toc = loaded<{ entries: TOCEntry[] }>();
  return {
    oninit: () => {
      const slug = encodeURIComponent(m.route.param('workSlug'));
      void load(w, () => reader.get<WorkDetail>('/works/' + slug)).then(() => setTitle(w.data?.title ?? 'Not found'));
      void load(toc, () => reader.get<{ entries: TOCEntry[] }>('/works/' + slug + '/toc'));
    },
    view: () => {
      if (w.loading && !w.data) return spinner();
      if (w.error) return w.error.status === 404
        ? emptyState('question-circle', 'Not found', ['This work does not exist or is not available. ', m(m.route.Link, { href: '/' }, 'Return home'), '.'])
        : alertBox('danger', w.error.message);
      const d = w.data!;
      const story = d.kind === 'short_story';
      const entries = toc.data?.entries ?? [];
      const storyEntry = story ? entries[0] : undefined;
      return m('.row.g-4', [
        m('.col-lg-8', [
          m('.d-flex.flex-wrap.gap-1.mb-2', [
            m('span.badge.text-bg-secondary', story ? 'Short story' : 'Novel'),
            m(m.route.Link, { href: (story ? '/stories' : '/novels') + '?genre=' + d.genre.slug, class: 'badge text-bg-light border text-decoration-none' }, d.genre.name),
            story ? null : m('span.badge.text-bg-light.border', STATUS_LABEL[d.publication_status]),
            m('span.badge.text-bg-light.border', { title: 'Age rating' }, d.age_rating === 'general' ? 'All ages' : d.age_rating === 'teen' ? 'Teen' : 'Mature'),
          ]),
          m('h1.display-6.rail-title.mb-1', d.title),
          d.subtitle ? m('p.lead', d.subtitle) : null,
          m('p.text-body-secondary', ['by ', m(m.route.Link, { href: '/authors/' + d.author.slug }, d.author.name)]),
          d.content_warnings.length ? alertBox('warning', [icon('exclamation-triangle'), ' Content warnings: ', d.content_warnings.join(', ')]) : null,
          m('.mb-4', d.full_synopsis.split(/\n{2,}/).map((para) => m('p', para))),
          m('.d-flex.flex-wrap.gap-2.mb-4', [
            story
              ? storyEntry
                ? storyEntry.access === 'upcoming'
                  ? m('span.btn.btn-secondary.disabled', { 'aria-disabled': 'true' }, ['Free ', formatDate(storyEntry.public_at)])
                  : m(m.route.Link, { href: entryHref('short_story', d.slug, storyEntry.slug), class: 'btn btn-primary btn-lg' }, [icon('book'), ' Read story'])
                : null
              : d.start
                ? m(m.route.Link, { href: entryHref('novel', d.slug, d.start.slug), class: 'btn btn-primary btn-lg' }, [icon('book'), ' Start reading'])
                : null,
            // Saving and following arrive with the reader library.
          ]),
          story && storyEntry ? m('p.text-body-secondary', [`${minutes(storyEntry.read_seconds)} · ${formatCount(storyEntry.word_count)} words`, accessBadge(storyEntry)]) : null,
          d.tags.length ? m('.mb-4', [m('h2.visually-hidden', 'Tags'), d.tags.map((t) => m('span.badge.rounded-pill.text-bg-light.border.me-1', t.label))]) : null,
          story ? null : m('section', { 'aria-labelledby': 'toc-h' }, [
            m('h2.h4.rail-title', { id: 'toc-h' }, 'Contents'),
            toc.loading && !toc.data ? spinner() : entries.length ? tocList(entries, d.slug) : emptyState('list-ol', 'No chapters yet', ''),
          ]),
        ]),
        m('aside.col-lg-4', [
          m('.card.mb-3', m('.card-body.small', [
            m('dl.row.mb-0', [
              m('dt.col-6', 'Words (public)'), m('dd.col-6', formatCount(d.word_count)),
              story ? null : [m('dt.col-6', 'Public chapters'), m('dd.col-6', d.public_entry_count)],
              m('dt.col-6', 'First published'), m('dd.col-6', formatDate(d.first_public_at)),
              d.last_public_update_at ? [m('dt.col-6', 'Last update'), m('dd.col-6', formatDate(d.last_public_update_at))] : null,
              m('dt.col-6', 'Rating'), m('dd.col-6', d.rating.state === 'insufficient_data' ? 'Insufficient data' : `${d.rating.average?.toFixed(1)} (${d.rating.count})`),
            ]),
          ])),
          d.next_release ? m('.card.mb-3', m('.card-body.small', [
            m('h2.h6', 'Next release'),
            m('p.mb-1', d.next_release.title),
            m('p.mb-0', d.next_release.access === 'early'
              ? ['Early access for supporters now; free for everyone on ', m('strong', formatDate(d.next_release.public_at, true)), '.']
              : ['Free for everyone on ', m('strong', formatDate(d.next_release.public_at, true)), '.']),
          ])) : null,
          d.collections.length ? m('.card.mb-3', m('.card-body.small', [
            m('h2.h6', 'In collections'),
            m('ul.mb-0', d.collections.map((c) => m('li', m(m.route.Link, { href: '/collections/' + c.slug }, c.name)))),
          ])) : null,
          m('.card', m('.card-body.small', [m('h2.h6', ['About ', d.author.name]), m('p.mb-0', d.author.bio || '—')])),
        ]),
      ]);
    },
  };
}
