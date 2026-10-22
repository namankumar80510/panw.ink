// §4.4 WorkCard: cover, title, author, format, genre, status, length and a
// content-warning flag. No invented review scores.
import m from 'mithril';
import type { WorkCard } from '../api/types';
import { formatCount, STATUS_LABEL, workHref } from '../utils/format';
import { icon } from './ui';

export const WorkCardView: m.Component<{ work: WorkCard; compact?: boolean }> = {
  view: ({ attrs: { work: w, compact } }) => {
    const story = w.kind === 'short_story';
    return m('article.card.h-100', [
      m('.card-body.d-flex.flex-column', [
        m('.d-flex.flex-wrap.gap-1.mb-2.small', [
          m('span.badge.text-bg-secondary', story ? 'Short story' : 'Novel'),
          m('span.badge.text-bg-light.border', w.genre.name),
          story ? null : m('span.badge.text-bg-light.border', STATUS_LABEL[w.publication_status] ?? w.publication_status),
          w.age_rating === 'mature' ? m('span.badge.text-bg-dark', 'Mature') : null,
          w.content_warnings.length ? m('span.badge.text-bg-warning', { title: 'Content warnings: ' + w.content_warnings.join(', ') }, [icon('exclamation-triangle'), ' Warnings']) : null,
        ]),
        m('h3.h5.card-title.rail-title.mb-1', m(m.route.Link, { href: workHref(w.slug), class: 'stretched-link text-decoration-none' }, w.title)),
        m('p.small.text-body-secondary.mb-2', ['by ', w.author.name]),
        compact ? null : m('p.card-text.small.flex-grow-1', w.synopsis),
        m('p.small.text-body-secondary.mb-0.mt-auto', story
          ? `${w.read_minutes} min read · ${formatCount(w.word_count)} words`
          : `${w.public_entry_count} ${w.public_entry_count === 1 ? 'chapter' : 'chapters'} · ${formatCount(w.word_count)} words`),
      ]),
    ]);
  },
};

export function cardGrid(works: WorkCard[], compact = false): m.Vnode {
  return m('.row.row-cols-1.row-cols-sm-2.row-cols-lg-3.row-cols-xl-4.g-3',
    works.map((w) => m('.col', { key: w.id }, m(WorkCardView, { work: w, compact }))));
}
