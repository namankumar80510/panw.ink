// ReadingShell (§4.4, §4.5): distraction-free text with the reader's width,
// size, leading, family and theme; previous/next; a contents drawer; arrow-key
// navigation; and plain-language lock pages for early or upcoming entries.
//
// body_html is sanitized on the server when the revision is written
// (internal/render: Goldmark without raw HTML + bluemonday allowlist), which
// is the only reason m.trust is acceptable here.
import m from 'mithril';
import Offcanvas from 'bootstrap/js/dist/offcanvas';
import { ApiError } from '../api/http';
import { reader } from '../api/reader';
import type { EntryBody, EntryRef, LockedDetails, TOCEntry } from '../api/types';
import { alertBox, emptyState, icon, spinner } from '../components/ui';
import { session } from '../stores/session';
import { entryHref, formatDate, minutes, workHref } from '../utils/format';
import { loginHref } from '../utils/redirect';
import { readJSON, writeJSON } from '../utils/storage';
import { load, loaded } from '../utils/load';
import { setTitle } from '../utils/title';
import { accessBadge } from './work';
import { PrefsCard } from './account';

type Kind = 'novel' | 'short_story';

function useOffcanvas() {
  let inst: Offcanvas | null = null;
  return {
    attrs: {
      oncreate: (v: m.VnodeDOM) => { inst = Offcanvas.getOrCreateInstance(v.dom as HTMLElement); },
      onremove: () => { inst?.dispose(); inst = null; },
    },
    show: () => inst?.show(),
    hide: () => inst?.hide(),
  };
}

function navButton(kind: Kind, workSlug: string, ref: EntryRef | null, dir: 'prev' | 'next'): m.Children {
  if (!ref) return m('span');
  const label = dir === 'prev' ? [icon('arrow-left'), ' Previous'] : ['Next ', icon('arrow-right')];
  return m(m.route.Link, {
    href: entryHref(kind, workSlug, ref.slug),
    class: 'btn btn-outline-primary',
    rel: dir,
    title: ref.title,
  }, [label, ref.access !== 'public' ? m('span.visually-hidden', ` (${ref.access === 'early' ? 'early access' : 'not yet released'})`) : null]);
}

// Local, non-sensitive "finished" marks for short stories (§4.5). Account
// sync and qualified-completion analytics arrive with later phases.
const FINISHED_KEY = 'pawn.finished.v1';
function finished(): Record<string, string> { return readJSON<Record<string, string>>(FINISHED_KEY, {}); }

export function ReaderPage(kind: Kind): () => m.Component {
  return () => {
    const e = loaded<EntryBody>();
    const toc = loaded<{ entries: TOCEntry[] }>();
    const tocDrawer = useOffcanvas();
    const prefsDrawer = useOffcanvas();
    let locked: LockedDetails | null = null;
    let lockCode = '';
    const workSlug = m.route.param('workSlug');

    const onKey = (ev: KeyboardEvent) => {
      if (ev.altKey || ev.ctrlKey || ev.metaKey || ev.shiftKey) return;
      const t = ev.target as HTMLElement;
      if (t && (t.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(t.tagName))) return;
      const ref = ev.key === 'ArrowLeft' ? (e.data?.previous ?? locked?.entry.previous) : ev.key === 'ArrowRight' ? (e.data?.next ?? locked?.entry.next) : null;
      if (ref) { ev.preventDefault(); m.route.set(entryHref(kind, workSlug, ref.slug)); }
    };

    const openTOC = () => {
      if (!toc.data && !toc.error) void load(toc, () => reader.get('/works/' + encodeURIComponent(workSlug) + '/toc'));
      tocDrawer.show();
    };

    return {
      oninit: () => {
        const path = kind === 'short_story'
          ? `/stories/${encodeURIComponent(workSlug)}/read`
          : `/works/${encodeURIComponent(workSlug)}/entries/${encodeURIComponent(m.route.param('entrySlug'))}`;
        void load(e, () => reader.get<EntryBody>(path)).then(() => {
          if (e.error instanceof ApiError && (e.error.code === 'EARLY_ACCESS_REQUIRED' || e.error.code === 'ENTRY_UPCOMING')) {
            locked = e.error.details as LockedDetails;
            lockCode = e.error.code;
          }
          const t = e.data ?? locked?.entry;
          if (t) setTitle(kind === 'short_story' ? t.title : `${t.title} · ${'work_title' in t ? t.work_title : ''}`);
          else setTitle('Not found');
        });
        document.addEventListener('keydown', onKey);
      },
      onremove: () => document.removeEventListener('keydown', onKey),
      view: () => {
        if (e.loading && !e.data) return spinner('Loading text');
        const meta = e.data ?? locked?.entry;
        if (!meta) {
          return e.error?.status === 404
            ? emptyState('question-circle', 'Not found', ['This text does not exist or is not available. ', m(m.route.Link, { href: '/' }, 'Return home'), '.'])
            : alertBox('danger', e.error?.message ?? 'Could not load this text.');
        }
        const previous = e.data?.previous ?? locked?.entry.previous ?? null;
        const next = e.data?.next ?? locked?.entry.next ?? null;
        const story = kind === 'short_story';
        const done = story && e.data ? finished()[e.data.work_id] : undefined;

        return [
          m('nav.d-flex.flex-wrap.align-items-center.gap-2.mb-4.small', { 'aria-label': 'Reading tools' }, [
            m(m.route.Link, { href: workHref(workSlug), class: 'link-secondary text-decoration-none me-auto' }, [icon('chevron-left'), ' ', meta.work_title]),
            story ? null : m('button.btn.btn-sm.btn-outline-secondary', { type: 'button', onclick: openTOC, 'aria-controls': 'tocDrawer' }, [icon('list-ol'), ' Contents']),
            m('button.btn.btn-sm.btn-outline-secondary', { type: 'button', onclick: () => prefsDrawer.show(), 'aria-controls': 'prefsDrawer' }, [icon('type'), ' Text settings']),
          ]),

          m('article.reader-text', { 'aria-labelledby': 'entry-title' }, [
            m('header.mb-4', [
              story ? null : m('p.text-body-secondary.small.mb-1.text-uppercase', [
                meta.volume ? `Volume ${meta.volume.number} · ` : '', `Chapter ${meta.ordinal}`,
              ]),
              m('h1.rail-title', { id: 'entry-title' }, meta.title),
              m('p.text-body-secondary.small', [
                'by ', m(m.route.Link, { href: '/authors/' + meta.author.slug }, meta.author.name),
                e.data ? ` · ${minutes(e.data.read_seconds)}` : '',
                e.data?.revised_at ? ` · Updated ${formatDate(e.data.revised_at)}` : '',
              ]),
            ]),
            e.data
              ? m('.entry-body', m.trust(e.data.body_html))
              : m('section.card.my-4', { 'aria-live': 'polite' }, m('.card-body', [
                  m('h2.h5', lockCode === 'ENTRY_UPCOMING' ? 'Not released yet' : 'Early access'),
                  m('p', ['This ', story ? 'story' : 'chapter', ' becomes free for everyone on ', m('strong', formatDate(locked!.public_at, true)), '.']),
                  lockCode === 'EARLY_ACCESS_REQUIRED' ? [
                    m('p', 'Patreon supporters can read it now. All main fiction eventually releases free; early access lasts only until the public date.'),
                    m('.d-flex.flex-wrap.gap-2', [
                      session.user
                        ? m(m.route.Link, { href: '/settings/patreon', class: 'btn btn-primary' }, 'Connect Patreon')
                        : m(m.route.Link, { href: loginHref(), class: 'btn btn-primary' }, 'Sign in to use early access'),
                      m(m.route.Link, { href: '/support', class: 'btn btn-outline-secondary' }, 'How early access works'),
                    ]),
                  ] : m('p.mb-0.text-body-secondary', 'Nobody can read it before its release time, including supporters.'),
                ])),
          ]),

          m('nav.reader-text.d-flex.justify-content-between.align-items-center.gap-2.border-top.pt-4.mt-5', { 'aria-label': story ? 'Story navigation' : 'Chapter navigation' }, story
            ? [
                m(m.route.Link, { href: workHref(workSlug), class: 'btn btn-outline-secondary' }, [icon('chevron-left'), ' About this story']),
                e.data ? m('button.btn', {
                  type: 'button', class: done ? 'btn-success' : 'btn-outline-success', 'aria-pressed': done ? 'true' : 'false',
                  onclick: () => {
                    const f = finished();
                    if (f[e.data!.work_id]) delete f[e.data!.work_id]; else f[e.data!.work_id] = new Date().toISOString();
                    writeJSON(FINISHED_KEY, f);
                  },
                }, done ? [icon('check2-circle'), ' Finished'] : 'Mark finished') : null,
              ]
            : [
                navButton(kind, workSlug, previous, 'prev'),
                m('button.btn.btn-link.btn-sm', { type: 'button', onclick: openTOC }, 'Contents'),
                navButton(kind, workSlug, next, 'next'),
              ]),
          story ? null : m('p.reader-text.small.text-body-secondary.text-center.mt-2.d-none.d-md-block', 'Tip: use ← and → to move between chapters.'),

          m('.offcanvas.offcanvas-start', { id: 'tocDrawer', tabindex: -1, 'aria-labelledby': 'tocDrawerLabel', ...tocDrawer.attrs }, [
            m('.offcanvas-header', [m('h2.offcanvas-title.h5', { id: 'tocDrawerLabel' }, 'Contents'), m('button.btn-close', { type: 'button', 'aria-label': 'Close', onclick: tocDrawer.hide })]),
            m('.offcanvas-body', toc.loading && !toc.data ? spinner() : toc.error ? alertBox('danger', toc.error.message) : m('ol.list-unstyled', (toc.data?.entries ?? []).map((t) =>
              m('li.mb-2', { key: t.id }, [
                t.access === 'upcoming'
                  ? m('span.text-body-secondary', `${t.ordinal}. ${t.title}`)
                  : m(m.route.Link, {
                      href: entryHref(kind, workSlug, t.slug), onclick: tocDrawer.hide,
                      class: t.slug === meta.slug ? 'fw-bold' : '', 'aria-current': t.slug === meta.slug ? 'page' : undefined,
                    }, `${t.ordinal}. ${t.title}`),
                accessBadge(t),
              ])))),
          ]),
          m('.offcanvas.offcanvas-end', { id: 'prefsDrawer', tabindex: -1, 'aria-labelledby': 'prefsDrawerLabel', ...prefsDrawer.attrs }, [
            m('.offcanvas-header', [m('h2.offcanvas-title.h5', { id: 'prefsDrawerLabel' }, 'Text settings'), m('button.btn-close', { type: 'button', 'aria-label': 'Close', onclick: prefsDrawer.hide })]),
            m('.offcanvas-body', m(PrefsCard)),
          ]),
        ];
      },
    };
  };
}
