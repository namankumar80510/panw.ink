// Studio screens (§4.3, §11). Listings and safe metadata edits use Collar's
// native API directly; creates, ordering, tags, scheduling and validation use
// the /studio commands.
import m from 'mithril';
import { collar } from '../api/collar';
import { ApiError } from '../api/http';
import { reader } from '../api/reader';
import type { TagRef } from '../api/types';
import { alertBox, emptyState, errorText, field, icon, spinner, submitButton } from '../components/ui';
import { toast } from '../components/toasts';
import { formatCount, formatDate } from '../utils/format';
import { setTitle } from '../utils/title';
import { browserZone, wallToUTC } from '../utils/tz';
import {
  releaseBadge, releaseOf, studioHeader, uploadImage,
  type AssetRow, type CollectionRow, type PenNameRow, type StudioEntryRow, type VolumeRow, type WorkRow,
} from './common';

interface Genre { slug: string; name: string; id?: string }

let genreCache: Array<{ id: string; slug: string; name: string }> | null = null;
async function genres() {
  genreCache ??= (await collar.list<{ id: string; slug: string; name: string }>('genres', { sort: 'display_order', perPage: 100 })).items;
  return genreCache;
}

// ---- overview -----------------------------------------------------------------

export function Overview(): m.Component {
  const s = { works: null as WorkRow[] | null, upcoming: [] as StudioEntryRow[], drafts: 0, error: '' };
  return {
    oninit: async () => {
      setTitle('Studio');
      try {
        const [w, up, dr] = await Promise.all([
          collar.list<WorkRow>('works', { sort: '-updated_at', perPage: 100 }),
          collar.list<StudioEntryRow>('studio_entries', { filter: 'workflow = "scheduled" && public_at > @now', sort: 'public_at', perPage: 10 }),
          collar.list<StudioEntryRow>('studio_entries', { filter: 'workflow = "draft"', perPage: 1 }),
        ]);
        s.works = w.items; s.upcoming = up.items; s.drafts = dr.totalItems ?? 0;
      } catch (e) { s.error = errorText(e); }
      m.redraw();
    },
    view: () => [
      studioHeader('Studio', m(m.route.Link, { href: '/studio/works/new', class: 'btn btn-primary' }, [icon('plus-lg'), ' New work'])),
      s.error ? alertBox('danger', s.error) : s.works === null ? spinner() : [
        m('.row.g-3.mb-4', [
          ['Works', s.works.length, 'journal-text'], ['Listed', s.works.filter((w) => w.visibility === 'listed').length, 'eye'],
          ['Draft entries', s.drafts, 'pencil'], ['Upcoming releases', s.upcoming.length, 'calendar-event'],
        ].map(([label, n, ic]) => m('.col-6.col-lg-3', m('.card', m('.card-body', [m('.text-body-secondary.small', [icon(ic as string), ' ', label]), m('.fs-3', String(n))]))))),
        m('h2.h5', 'Upcoming releases'),
        s.upcoming.length
          ? m('ul.list-group.mb-4', s.upcoming.map((e) => m('li.list-group-item.d-flex.justify-content-between.align-items-center', [
              m(m.route.Link, { href: `/studio/entries/${e.id}/schedule` }, e.title), releaseBadge(e)])))
          : emptyState('calendar', 'Nothing scheduled', 'Schedule a chapter or story from its editor.'),
        m('h2.h5', 'Recently edited works'),
        worksTable(s.works.slice(0, 8)),
      ],
    ],
  };
}

function worksTable(works: WorkRow[]): m.Children {
  if (!works.length) return emptyState('journal-plus', 'No works yet', m(m.route.Link, { href: '/studio/works/new' }, 'Create your first work'));
  return m('.table-responsive', m('table.table.align-middle', [
    m('thead', m('tr', ['Title', 'Format', 'Status', 'Visibility', 'Public words', 'Updated', ''].map((h) => m('th', { scope: 'col' }, h)))),
    m('tbody', works.map((w) => m('tr', { key: w.id }, [
      m('td', [m(m.route.Link, { href: `/studio/works/${w.id}/edit` }, w.title), w.expand?.pen_name_id ? m('div.small.text-body-secondary', w.expand.pen_name_id.name) : null]),
      m('td', w.kind === 'novel' ? 'Novel' : 'Short story'),
      m('td', w.publication_status),
      m('td', m('span.badge', { class: w.visibility === 'listed' ? 'text-bg-success' : w.visibility === 'hidden' ? 'text-bg-danger' : 'text-bg-secondary' }, w.visibility)),
      m('td', formatCount(w.public_word_count)),
      m('td.small', formatDate(w.updated_at)),
      m('td.text-end', m(m.route.Link, { href: w.kind === 'novel' ? `/studio/works/${w.id}/entries` : `/studio/works/${w.id}/entries`, class: 'btn btn-sm btn-outline-primary' }, w.kind === 'novel' ? 'Chapters' : 'Text')),
    ]))),
  ]));
}

// ---- works list ---------------------------------------------------------------

export function WorksList(): m.Component {
  const s = { items: null as WorkRow[] | null, error: '', kind: '', visibility: '', status: '' };
  const load = async () => {
    const f: string[] = [];
    if (s.kind) f.push(`kind = "${s.kind}"`);
    if (s.visibility) f.push(`visibility = "${s.visibility}"`);
    if (s.status) f.push(`publication_status = "${s.status}"`);
    try { s.items = (await collar.list<WorkRow>('works', { filter: f.join(' && '), sort: '-updated_at', expand: 'pen_name_id', perPage: 100 })).items; } catch (e) { s.error = errorText(e); }
    m.redraw();
  };
  const sel = (label: string, key: 'kind' | 'visibility' | 'status', opts: Array<[string, string]>) => m('.col-auto', [
    m('label.form-label.small', { for: 'f-' + key }, label),
    m('select.form-select.form-select-sm', { id: 'f-' + key, value: s[key], onchange: (e: Event) => { s[key] = (e.target as HTMLSelectElement).value; void load(); } },
      opts.map(([v, l]) => m('option', { value: v }, l))),
  ]);
  return {
    oninit: () => { setTitle('Works'); void load(); },
    view: () => [
      studioHeader('Works', m(m.route.Link, { href: '/studio/works/new', class: 'btn btn-primary' }, [icon('plus-lg'), ' New work'])),
      m('.row.g-2.mb-3', [
        sel('Format', 'kind', [['', 'Any'], ['novel', 'Novels'], ['short_story', 'Short stories']]),
        sel('Visibility', 'visibility', [['', 'Any'], ['draft', 'Draft'], ['listed', 'Listed'], ['unlisted', 'Unlisted'], ['hidden', 'Hidden by moderation']]),
        sel('Status', 'status', [['', 'Any'], ['ongoing', 'Ongoing'], ['completed', 'Completed'], ['hiatus', 'Hiatus'], ['cancelled', 'Cancelled']]),
      ]),
      s.error ? alertBox('danger', s.error) : s.items === null ? spinner() : worksTable(s.items),
    ],
  };
}

// ---- new work wizard (§11.1) -----------------------------------------------------

export function NewWork(): m.Component {
  const s = {
    step: 1, kind: '' as '' | 'novel' | 'short_story', pens: null as PenNameRow[] | null, pen: '',
    title: '', slug: '', slugTouched: false, genre: '', synopsis: '', language: 'en', rating: 'teen', warnings: '', tags: new Set<string>(),
    withVolume: false, genres: [] as Genre[], allTags: [] as TagRef[], busy: false, error: '', fields: {} as Record<string, string>,
  };
  const slugify = (t: string) => t.toLowerCase().normalize('NFKD').replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 160);
  const create = async () => {
    s.busy = true; s.error = ''; s.fields = {};
    try {
      const w = await reader.post<{ id: string; entry_id: string | null }>('/studio/works', {
        pen_name_id: s.pen, kind: s.kind, title: s.title, slug: s.slug, genre: s.genre, synopsis: s.synopsis,
        language_code: s.language, age_rating: s.rating, tags: [...s.tags],
        content_warnings: s.warnings.split(',').map((x) => x.trim()).filter(Boolean),
      });
      if (s.kind === 'short_story') {
        m.route.set(`/studio/entries/${w.entry_id}/edit`);
        return;
      }
      let volume: string | null = null;
      if (s.withVolume) volume = (await reader.post<{ id: string }>(`/studio/works/${w.id}/volumes`, { title: 'Volume 1' })).id;
      const ch = await reader.post<{ id: string }>(`/studio/works/${w.id}/entries`, { title: 'Chapter 1', volume_id: volume });
      m.route.set(`/studio/entries/${ch.id}/edit`);
    } catch (e) {
      if (e instanceof ApiError && Object.keys(e.fields).length) { s.fields = e.fields; s.step = 3; } else s.error = errorText(e);
    } finally { s.busy = false; m.redraw(); }
  };
  return {
    oninit: async () => {
      setTitle('New work');
      try {
        const [p, g, t] = await Promise.all([collar.list<PenNameRow>('pen_names', { perPage: 100, sort: 'name' }), reader.get<Genre[]>('/genres'), reader.get<TagRef[]>('/tags')]);
        s.pens = p.items.filter((x) => x.state === 'active'); s.genres = g; s.allTags = t;
        if (s.pens.length === 1) s.pen = s.pens[0]!.id;
      } catch (e) { s.error = errorText(e); }
      m.redraw();
    },
    view: () => [
      studioHeader('New work', null, [['/studio/works', 'Works']]),
      m('ol.list-inline.small.text-body-secondary.mb-4', ['Format', 'Pen name', 'Details'].map((l, i) =>
        m('li.list-inline-item', { class: s.step === i + 1 ? 'fw-bold text-body' : '', 'aria-current': s.step === i + 1 ? 'step' : undefined }, `${i + 1}. ${l}`))),
      s.error ? alertBox('danger', s.error) : null,
      s.step === 1 ? m('fieldset', [
        m('legend.h5', 'What are you writing?'),
        m('.row.g-3', ([['novel', 'Novel', 'A sequence of chapters, optionally in volumes, released over time.', 'book'],
          ['short_story', 'Short story', 'One complete text. No chapters, no serialization.', 'file-earmark-text']] as const).map(([k, l, d, ic]) =>
          m('.col-md-6', m('label.card.h-100', { class: s.kind === k ? 'border-primary' : '' }, m('.card-body', [
            m('input.form-check-input.me-2', { type: 'radio', name: 'kind', checked: s.kind === k, onchange: () => (s.kind = k) }),
            m('span.h5', [icon(ic), ' ', l]), m('p.mb-0.mt-2.small', d),
          ]))))),
        m('p.small.text-body-secondary.mt-2', 'The format cannot change after publication.'),
        m('button.btn.btn-primary.mt-2', { type: 'button', disabled: !s.kind, onclick: () => (s.step = 2) }, 'Continue'),
      ]) : null,
      s.step === 2 ? (s.pens === null ? spinner() : m('fieldset', [
        m('legend.h5', 'Which pen name?'),
        s.pens.length === 0
          ? alertBox('info', ['You have no pen names yet. ', m(m.route.Link, { href: '/studio/authors' }, 'Create one first'), '.'])
          : m('.list-group.mb-3', s.pens.map((p) => m('label.list-group-item', [
              m('input.form-check-input.me-2', { type: 'radio', name: 'pen', checked: s.pen === p.id, onchange: () => (s.pen = p.id) }), p.name]))),
        m('.d-flex.gap-2', [m('button.btn.btn-outline-secondary', { type: 'button', onclick: () => (s.step = 1) }, 'Back'),
          m('button.btn.btn-primary', { type: 'button', disabled: !s.pen, onclick: () => (s.step = 3) }, 'Continue')]),
      ])) : null,
      s.step === 3 ? m('form.row.g-4', { novalidate: true, onsubmit: (e: Event) => { e.preventDefault(); void create(); } }, [
        m('.col-lg-7', [
          field({ id: 'title', label: 'Title', value: s.title, required: true, maxlength: 240, error: s.fields.title,
            oninput: (v) => { s.title = v; if (!s.slugTouched) s.slug = slugify(v); } }),
          field({ id: 'slug', label: 'Address', value: s.slug, error: s.fields.slug, help: `pawn.ink/fiction/${s.slug || '…'} — permanent once published.`,
            oninput: (v) => { s.slug = v.toLowerCase(); s.slugTouched = true; } }),
          m('.mb-3', [m('label.form-label', { for: 'genre' }, 'Primary genre'),
            m('select.form-select', { id: 'genre', value: s.genre, class: s.fields.genre ? 'is-invalid' : '', onchange: (e: Event) => (s.genre = (e.target as HTMLSelectElement).value) },
              [m('option', { value: '' }, 'Choose…'), ...s.genres.map((g) => m('option', { value: g.slug }, g.name))]),
            s.fields.genre ? m('.invalid-feedback', s.fields.genre) : null]),
          m('.mb-3', [m('label.form-label', { for: 'synopsis' }, 'Synopsis'),
            m('textarea.form-control', { id: 'synopsis', rows: 6, maxlength: 4000, value: s.synopsis, oninput: (e: Event) => (s.synopsis = (e.target as HTMLTextAreaElement).value) }),
            m('.form-text', `${s.synopsis.length} / 4000`)]),
          m('fieldset.mb-3', [m('legend.form-label.fs-6', 'Tags'), m('div', s.allTags.map((t) => m('.form-check.form-check-inline', [
            m('input.form-check-input', { type: 'checkbox', id: 'tag-' + t.slug, checked: s.tags.has(t.slug), onchange: () => (s.tags.has(t.slug) ? s.tags.delete(t.slug) : s.tags.add(t.slug)) }),
            m('label.form-check-label', { for: 'tag-' + t.slug }, t.label)]))), s.fields.tags ? m('.text-danger.small', s.fields.tags) : null]),
          m('.row', [
            m('.col-sm-6.mb-3', [m('label.form-label', { for: 'rating' }, 'Age rating'), m('select.form-select', { id: 'rating', value: s.rating, onchange: (e: Event) => (s.rating = (e.target as HTMLSelectElement).value) },
              [['general', 'All ages'], ['teen', 'Teen'], ['mature', 'Mature']].map(([v, l]) => m('option', { value: v }, l)))]),
            m('.col-sm-6', field({ id: 'language', label: 'Language code', value: s.language, oninput: (v) => (s.language = v), error: s.fields.language_code })),
          ]),
          field({ id: 'warnings', label: 'Content warnings (comma separated)', value: s.warnings, oninput: (v) => (s.warnings = v), error: s.fields.content_warnings }),
          s.kind === 'novel' ? m('.form-check.mb-3', [m('input.form-check-input', { type: 'checkbox', id: 'withvol', checked: s.withVolume, onchange: () => (s.withVolume = !s.withVolume) }),
            m('label.form-check-label', { for: 'withvol' }, 'Start with “Volume 1” (volumes are optional)')]) : null,
          m('.d-flex.gap-2', [m('button.btn.btn-outline-secondary', { type: 'button', onclick: () => (s.step = 2) }, 'Back'),
            submitButton(s.kind === 'novel' ? 'Create novel and write chapter 1' : 'Create story and start writing', s.busy)]),
        ]),
        m('.col-lg-5', m('.card', m('.card-body', [
          m('.small.text-body-secondary.mb-2', 'Preview card'),
          m('span.badge.text-bg-secondary.me-1', s.kind === 'novel' ? 'Novel' : 'Short story'),
          s.genre ? m('span.badge.text-bg-light.border', s.genres.find((g) => g.slug === s.genre)?.name) : null,
          m('h3.h5.mt-2.rail-title', s.title || 'Untitled'),
          m('p.small.text-body-secondary', ['by ', s.pens?.find((p) => p.id === s.pen)?.name ?? '']),
          m('p.small', s.synopsis.slice(0, 300) || 'Your synopsis appears here.'),
        ]))),
      ]) : null,
    ],
  };
}

// ---- work settings --------------------------------------------------------------

export function WorkEdit(): m.Component {
  const id = m.route.param('id');
  const s = {
    w: null as WorkRow | null, error: '', busy: false, fields: {} as Record<string, string>, warnings: '',
    allTags: [] as TagRef[], tags: new Set<string>(), genres: [] as Array<{ id: string; slug: string; name: string }>, coverBusy: false, reason: '',
  };
  const load = async () => {
    try {
      const [w, wt, at, g] = await Promise.all([
        collar.view<WorkRow>('works', id, { expand: 'pen_name_id' }),
        collar.list<{ tag_id: string }>('work_tags', { filter: `work_id = ${id}`, perPage: 50 }),
        reader.get<TagRef[]>('/tags'),
        genres(),
      ]);
      const tagRows = await collar.list<{ id: string; slug: string }>('tags', { perPage: 200 });
      const byID = new Map(tagRows.items.map((t) => [t.id, t.slug]));
      s.w = w; s.allTags = at; s.genres = g; s.warnings = (w.content_warnings_json ?? []).join(', ');
      s.tags = new Set(wt.items.map((x) => byID.get(x.tag_id)).filter(Boolean) as string[]);
      setTitle(w.title);
    } catch (e) { s.error = errorText(e); }
    m.redraw();
  };
  const save = async () => {
    const w = s.w!;
    s.busy = true; s.fields = {};
    try {
      s.w = { ...await collar.update<WorkRow>('works', id, {
        title: w.title, subtitle: w.subtitle || null, synopsis: w.synopsis, primary_genre_id: Number(w.primary_genre_id), age_rating: w.age_rating,
        language_code: w.language_code, visibility: w.visibility, publication_status: w.publication_status,
        content_warnings_json: s.warnings.split(',').map((x) => x.trim()).filter(Boolean),
      }), expand: w.expand };
      await reader.put(`/studio/works/${id}/tags`, { tags: [...s.tags] });
      toast('Saved.', 'success');
    } catch (e) {
      const d = (e as ApiError).details as Record<string, string[]> | null;
      if (d && typeof d === 'object') for (const [k, v] of Object.entries(d)) s.fields[k] = Array.isArray(v) ? v[0]! : String(v);
      toast(errorText(e), 'danger');
    } finally { s.busy = false; m.redraw(); }
  };
  const onCover = async (ev: Event) => {
    const file = (ev.target as HTMLInputElement).files?.[0];
    if (!file || !s.w) return;
    s.coverBusy = true; m.redraw();
    try {
      const a = await uploadImage(file, `Cover of ${s.w.title}`);
      s.w = { ...await collar.update<WorkRow>('works', id, { cover_asset_id: Number(a.id) }), expand: s.w.expand };
      toast('Cover updated.', 'success');
    } catch (e) { toast(errorText(e), 'danger'); } finally { s.coverBusy = false; m.redraw(); }
  };
  return {
    oninit: load,
    view: () => {
      if (s.error) return alertBox('danger', s.error);
      if (!s.w) return spinner();
      const w = s.w;
      const hidden = w.visibility === 'hidden';
      const set = (k: keyof WorkRow) => (v: string) => ((w as unknown as Record<string, string>)[k] = v);
      return [
        studioHeader(w.title, [
          m(m.route.Link, { href: `/studio/works/${id}/entries`, class: 'btn btn-outline-primary' }, [icon('list-ol'), w.kind === 'novel' ? ' Chapters' : ' Story text']),
          w.visibility !== 'draft' ? m('a.btn.btn-outline-secondary', { href: `/fiction/${w.slug}`, target: '_blank', rel: 'noopener' }, [icon('box-arrow-up-right'), ' View']) : null,
        ], [['/studio/works', 'Works']]),
        hidden ? m('.alert.alert-danger', 'This work is hidden by moderation. It cannot be edited or published until a moderator restores it.') : null,
        m('form.row.g-4', { onsubmit: (e: Event) => { e.preventDefault(); void save(); } }, m('fieldset.row.g-4', { disabled: hidden }, [
          m('.col-lg-8', [
            field({ id: 'title', label: 'Title', value: w.title, oninput: set('title'), maxlength: 240, error: s.fields.title }),
            field({ id: 'subtitle', label: 'Subtitle', value: w.subtitle ?? '', oninput: set('subtitle'), maxlength: 240 }),
            m('.mb-3', [m('label.form-label', { for: 'synopsis' }, 'Synopsis'),
              m('textarea.form-control', { id: 'synopsis', rows: 7, maxlength: 4000, value: w.synopsis, oninput: (e: Event) => (w.synopsis = (e.target as HTMLTextAreaElement).value) })]),
            m('fieldset.mb-3', [m('legend.form-label.fs-6', 'Tags'), m('div', s.allTags.map((t) => m('.form-check.form-check-inline', [
              m('input.form-check-input', { type: 'checkbox', id: 'tag-' + t.slug, checked: s.tags.has(t.slug), onchange: () => (s.tags.has(t.slug) ? s.tags.delete(t.slug) : s.tags.add(t.slug)) }),
              m('label.form-check-label', { for: 'tag-' + t.slug }, t.label)])))]),
            field({ id: 'warnings', label: 'Content warnings (comma separated)', value: s.warnings, oninput: (v) => (s.warnings = v) }),
            submitButton('Save changes', s.busy),
          ]),
          m('.col-lg-4', [
            m('.mb-3', [m('label.form-label', { for: 'visibility' }, 'Visibility'),
              m('select.form-select', { id: 'visibility', value: w.visibility, onchange: (e: Event) => (w.visibility = (e.target as HTMLSelectElement).value) },
                [['draft', 'Draft — not visible'], ['listed', 'Listed in the catalogue'], ['unlisted', 'Unlisted — link only']].map(([v, l]) => m('option', { value: v }, l))),
              m('.form-text', 'Readers only see released entries, whatever the visibility.')]),
            m('.mb-3', [m('label.form-label', { for: 'genre' }, 'Primary genre'),
              m('select.form-select', { id: 'genre', value: w.primary_genre_id, onchange: (e: Event) => (w.primary_genre_id = (e.target as HTMLSelectElement).value) },
                s.genres.map((g) => m('option', { value: g.id }, g.name)))]),
            w.kind === 'novel' ? m('.mb-3', [m('label.form-label', { for: 'status' }, 'Status'),
              m('select.form-select', { id: 'status', value: w.publication_status, disabled: w.publication_status === 'completed', onchange: (e: Event) => (w.publication_status = (e.target as HTMLSelectElement).value) },
                [['ongoing', 'Ongoing'], ['hiatus', 'On hiatus'], ['cancelled', 'Discontinued'], ['completed', 'Completed']].map(([v, l]) => m('option', { value: v, disabled: v === 'completed' && w.publication_status !== 'completed' }, l))),
              w.publication_status !== 'completed' ? m('button.btn.btn-sm.btn-outline-success.mt-2', { type: 'button', onclick: () => completeWork(id, () => load()) }, 'Mark novel complete…') : null]) : null,
            m('.mb-3', [m('label.form-label', { for: 'rating' }, 'Age rating'), m('select.form-select', { id: 'rating', value: w.age_rating, onchange: (e: Event) => (w.age_rating = (e.target as HTMLSelectElement).value) },
              [['general', 'All ages'], ['teen', 'Teen'], ['mature', 'Mature']].map(([v, l]) => m('option', { value: v }, l)))]),
            m('.card.mb-3', m('.card-body', [
              m('h2.h6', 'Cover'),
              w.cover_asset_id ? m('img.img-fluid.rounded.mb-2', { src: `/api/v1/pawn/x/assets/${w.cover_asset_id}/card`, alt: `Cover of ${w.title}` }) : m('p.small.text-body-secondary', 'No cover yet.'),
              m('label.form-label.small', { for: 'cover' }, 'Upload JPEG, PNG or WebP (≤ 5 MB)'),
              m('input.form-control.form-control-sm', { id: 'cover', type: 'file', accept: 'image/jpeg,image/png,image/webp', disabled: s.coverBusy, onchange: onCover }),
              s.coverBusy ? m('.small.mt-1', { role: 'status' }, 'Checking image…') : null,
            ])),
            m('.card.border-warning', m('.card-body', [
              m('h2.h6', 'Archive'),
              m('p.small', 'Removes the work from the catalogue and reading. Nothing is deleted; you can list it again later.'),
              field({ id: 'archive-reason', label: 'Reason (for the audit log)', value: s.reason, oninput: (v) => (s.reason = v) }),
              m('button.btn.btn-sm.btn-outline-warning', { type: 'button', disabled: s.reason.trim().length < 3,
                onclick: async () => { try { await reader.post(`/studio/works/${id}/archive`, { reason: s.reason }); toast('Archived.', 'success'); await load(); } catch (e) { toast(errorText(e), 'danger'); } } }, 'Archive work'),
            ])),
          ]),
        ])),
      ];
    },
  };
}

async function completeWork(id: string, after: () => void) {
  try {
    await reader.post(`/studio/works/${id}/complete`, {});
    toast('Marked complete.', 'success');
    after();
  } catch (e) {
    if (e instanceof ApiError && e.code === 'UNRELEASED_ENTRIES') {
      const n = ((e.details as { unreleased?: unknown[] })?.unreleased ?? []).length;
      // Explicit, accessible confirmation (no browser dialogs).
      completeConfirm = { id, n, after };
      m.redraw();
      return;
    }
    toast(errorText(e), 'danger');
  }
}
let completeConfirm: { id: string; n: number; after: () => void } | null = null;

export const CompleteConfirm: m.Component = {
  view: () => completeConfirm ? m('.alert.alert-warning.position-fixed.bottom-0.start-50.translate-middle-x.mb-3.shadow', { role: 'alertdialog', 'aria-live': 'assertive', style: { zIndex: 1080, maxWidth: '36rem' } }, [
    m('p', `${completeConfirm.n} chapter(s) are still drafts. They stay private and are not deleted. Mark the novel complete anyway?`),
    m('.d-flex.gap-2', [
      m('button.btn.btn-warning', { type: 'button', onclick: async () => {
        const c = completeConfirm!; completeConfirm = null;
        try { await reader.post(`/studio/works/${c.id}/complete`, { force: true }); toast('Marked complete.', 'success'); c.after(); } catch (e) { toast(errorText(e), 'danger'); }
      } }, 'Mark complete'),
      m('button.btn.btn-outline-secondary', { type: 'button', onclick: () => (completeConfirm = null) }, 'Cancel'),
    ]),
  ]) : null,
};

// ---- chapters / story entry ---------------------------------------------------------

export function Entries(): m.Component {
  const id = m.route.param('id');
  const s = {
    w: null as WorkRow | null, entries: [] as StudioEntryRow[], volumes: [] as VolumeRow[], error: '', newTitle: '', newVolume: '', volTitle: '',
    dragging: '' as string, bulk: { anchor: '', interval: 7, lead: 0, rank: 1, preview: null as null | Array<{ entry_id: string; title: string; public_at: string; early_at: string | null; problem?: string }> },
    zone: browserZone(),
  };
  const load = async () => {
    try {
      const [w, e, v] = await Promise.all([
        collar.view<WorkRow>('works', id),
        collar.list<StudioEntryRow>('studio_entries', { filter: `work_id = ${id}`, sort: 'ordinal', perPage: 500 }),
        collar.list<VolumeRow>('volumes', { filter: `work_id = ${id}`, sort: 'number', perPage: 100 }),
      ]);
      s.w = w; s.entries = e.items; s.volumes = v.items;
      setTitle(w.kind === 'novel' ? `Chapters · ${w.title}` : w.title);
      if (w.kind === 'short_story' && e.items[0] && m.route.get().endsWith('/entries')) {
        m.route.set(`/studio/entries/${e.items[0].id}/edit`, {}, { replace: true });
      }
    } catch (err) { s.error = errorText(err); }
    m.redraw();
  };
  const reorder = async (entryID: string, position: number) => {
    try { await reader.post(`/studio/entries/${entryID}/reorder`, { position }); await load(); } catch (e) { toast(errorText(e), 'danger'); }
  };
  const bulk = async (apply: boolean) => {
    const anchor = wallToUTC(s.bulk.anchor, s.zone);
    if (!anchor) { toast('Choose the first public date.', 'danger'); return; }
    try {
      const res = await reader.post<{ items: typeof s.bulk.preview }>(`/studio/works/${id}/bulk-schedule`, {
        anchor: anchor.toISOString(), interval_days: s.bulk.interval, early_lead_days: s.bulk.lead,
        min_access_rank: s.bulk.lead > 0 ? s.bulk.rank : null, apply,
      });
      s.bulk.preview = res.items;
      if (apply) { toast('Chapters scheduled.', 'success'); s.bulk.preview = null; await load(); }
    } catch (e) {
      const items = (e instanceof ApiError ? (e.details as { items?: typeof s.bulk.preview })?.items : null) ?? null;
      if (items) s.bulk.preview = items;
      toast(errorText(e), 'danger');
    }
    m.redraw();
  };
  return {
    oninit: load,
    view: () => {
      if (s.error) return alertBox('danger', s.error);
      if (!s.w) return spinner();
      const vol = (vid: string | null) => s.volumes.find((v) => v.id === vid);
      return [
        studioHeader(s.w.kind === 'novel' ? 'Chapters' : 'Story text', [
          m(m.route.Link, { href: `/studio/works/${id}/edit`, class: 'btn btn-outline-secondary' }, [icon('gear'), ' Work settings']),
        ], [['/studio/works', 'Works'], [`/studio/works/${id}/edit`, s.w.title]]),
        s.entries.length === 0 ? emptyState('file-earmark-plus', 'No chapters yet', 'Add the first chapter below.') :
          m('.table-responsive', m('table.table.align-middle', { 'aria-describedby': 'reorder-help' }, [
            m('thead', m('tr', ['#', 'Title', 'Status', 'Draft', '', 'Order'].map((h) => m('th', { scope: 'col' }, h)))),
            m('tbody', s.entries.map((e, i) => m('tr', {
              key: e.id, draggable: true, class: s.dragging === e.id ? 'opacity-50' : '',
              ondragstart: (ev: DragEvent) => { s.dragging = e.id; ev.dataTransfer?.setData('text/plain', e.id); },
              ondragend: () => (s.dragging = ''),
              ondragover: (ev: DragEvent) => ev.preventDefault(),
              ondrop: (ev: DragEvent) => { ev.preventDefault(); const from = s.dragging; s.dragging = ''; if (from && from !== e.id) void reorder(from, i + 1); },
            }, [
              m('td.text-body-secondary', [m('i.bi.bi-grip-vertical.me-1', { 'aria-hidden': 'true' }), e.ordinal]),
              m('td', [m(m.route.Link, { href: `/studio/entries/${e.id}/edit` }, e.title), vol(e.volume_id) ? m('div.small.text-body-secondary', `Volume ${vol(e.volume_id)!.number}: ${vol(e.volume_id)!.title}`) : null]),
              m('td', releaseBadge(e)),
              m('td.small', e.draft_version ? `v${e.draft_version} · ${formatCount(e.draft_word_count ?? 0)} words` : '—'),
              m('td.text-nowrap', [
                m(m.route.Link, { href: `/studio/entries/${e.id}/edit`, class: 'btn btn-sm btn-outline-primary me-1' }, 'Edit'),
                m(m.route.Link, { href: `/studio/entries/${e.id}/schedule`, class: 'btn btn-sm btn-outline-secondary' }, releaseOf(e) === 'draft' ? 'Schedule' : 'Release'),
              ]),
              m('td.text-nowrap', s.w!.kind === 'novel' ? [
                m('button.btn.btn-sm.btn-light.me-1', { type: 'button', disabled: i === 0, 'aria-label': `Move ${e.title} up`, onclick: () => reorder(e.id, i) }, icon('arrow-up')),
                m('button.btn.btn-sm.btn-light', { type: 'button', disabled: i === s.entries.length - 1, 'aria-label': `Move ${e.title} down`, onclick: () => reorder(e.id, i + 2) }, icon('arrow-down')),
              ] : null),
            ]))),
          ])),
        m('p.small.text-body-secondary', { id: 'reorder-help' }, 'Drag rows or use the arrow buttons to reorder. Links, bookmarks and reading progress keep working.'),
        s.w.kind === 'novel' ? m('.row.g-4.mt-1', [
          m('.col-lg-6', m('.card', m('form.card-body', {
            onsubmit: async (ev: Event) => {
              ev.preventDefault();
              try {
                const ch = await reader.post<{ id: string }>(`/studio/works/${id}/entries`, { title: s.newTitle, volume_id: s.newVolume || null });
                m.route.set(`/studio/entries/${ch.id}/edit`);
              } catch (e) { toast(errorText(e), 'danger'); }
            },
          }, [
            m('h2.h6', 'Add chapter'),
            field({ id: 'new-title', label: 'Title', value: s.newTitle, oninput: (v) => (s.newTitle = v), required: true, maxlength: 240 }),
            s.volumes.length ? m('.mb-3', [m('label.form-label', { for: 'new-vol' }, 'Volume'), m('select.form-select', { id: 'new-vol', value: s.newVolume, onchange: (e: Event) => (s.newVolume = (e.target as HTMLSelectElement).value) },
              [m('option', { value: '' }, 'No volume'), ...s.volumes.map((v) => m('option', { value: v.id }, `Volume ${v.number}: ${v.title}`))])]) : null,
            m('button.btn.btn-primary', { type: 'submit', disabled: !s.newTitle.trim() }, 'Add and edit'),
            m('hr'),
            m('h2.h6', 'Add volume'),
            m('.input-group', [
              m('input.form-control', { 'aria-label': 'Volume title', value: s.volTitle, placeholder: 'Volume title', oninput: (e: Event) => (s.volTitle = (e.target as HTMLInputElement).value) }),
              m('button.btn.btn-outline-secondary', { type: 'button', disabled: !s.volTitle.trim(),
                onclick: async () => { try { await reader.post(`/studio/works/${id}/volumes`, { title: s.volTitle }); s.volTitle = ''; await load(); } catch (e) { toast(errorText(e), 'danger'); } } }, 'Add'),
            ]),
          ]))),
          m('.col-lg-6', m('.card', m('.card-body', [
            m('h2.h6', 'Bulk schedule unreleased chapters'),
            m('.row.g-2', [
              m('.col-sm-7', [m('label.form-label.small', { for: 'b-anchor' }, 'First public date'), m('input.form-control.form-control-sm', { id: 'b-anchor', type: 'datetime-local', value: s.bulk.anchor, oninput: (e: Event) => (s.bulk.anchor = (e.target as HTMLInputElement).value) })]),
              m('.col-sm-5', [m('label.form-label.small', { for: 'b-zone' }, 'Time zone'), m('input.form-control.form-control-sm', { id: 'b-zone', value: s.zone, oninput: (e: Event) => (s.zone = (e.target as HTMLInputElement).value) })]),
              m('.col-4', [m('label.form-label.small', { for: 'b-int' }, 'Every (days)'), m('input.form-control.form-control-sm', { id: 'b-int', type: 'number', min: 1, max: 60, value: s.bulk.interval, oninput: (e: Event) => (s.bulk.interval = Number((e.target as HTMLInputElement).value)) })]),
              m('.col-4', [m('label.form-label.small', { for: 'b-lead' }, 'Early lead (days)'), m('input.form-control.form-control-sm', { id: 'b-lead', type: 'number', min: 0, max: 30, value: s.bulk.lead, oninput: (e: Event) => (s.bulk.lead = Number((e.target as HTMLInputElement).value)) })]),
              m('.col-4', [m('label.form-label.small', { for: 'b-rank' }, 'Tier rank'), m('input.form-control.form-control-sm', { id: 'b-rank', type: 'number', min: 1, max: 100, disabled: s.bulk.lead === 0, value: s.bulk.rank, oninput: (e: Event) => (s.bulk.rank = Number((e.target as HTMLInputElement).value)) })]),
            ]),
            m('.d-flex.gap-2.mt-3', [m('button.btn.btn-sm.btn-outline-primary', { type: 'button', onclick: () => bulk(false) }, 'Preview dates'),
              s.bulk.preview ? m('button.btn.btn-sm.btn-primary', { type: 'button', onclick: () => bulk(true) }, 'Apply schedule') : null]),
            s.bulk.preview ? m('table.table.table-sm.small.mt-3', [m('thead', m('tr', [m('th', 'Chapter'), m('th', 'Early'), m('th', 'Public'), m('th', '')])),
              m('tbody', s.bulk.preview.map((p) => m('tr', { class: p.problem ? 'table-danger' : '' }, [m('td', p.title), m('td', p.early_at ? formatDate(p.early_at, true) : '—'), m('td', formatDate(p.public_at, true)), m('td', p.problem ?? '')])))]) : null,
          ]))),
        ]) : null,
      ];
    },
  };
}

// ---- pen names ------------------------------------------------------------------------

export function PenNames(): m.Component {
  const s = { items: null as PenNameRow[] | null, error: '', slug: '', name: '', bio: '', fields: {} as Record<string, string>, editing: null as PenNameRow | null };
  const load = async () => {
    try { s.items = (await collar.list<PenNameRow>('pen_names', { sort: 'name', perPage: 100 })).items; } catch (e) { s.error = errorText(e); }
    m.redraw();
  };
  return {
    oninit: () => { setTitle('Pen names'); void load(); },
    view: () => [
      studioHeader('Pen names'),
      m('p.text-body-secondary', 'Pen names are public identities. They are not separate accounts and should not imply different real people with invented biographies.'),
      s.error ? alertBox('danger', s.error) : s.items === null ? spinner() : m('.row.g-3.mb-4', s.items.map((p) => m('.col-md-6', m('.card.h-100', m('.card-body', s.editing?.id === p.id ? [
        field({ id: 'e-name', label: 'Name', value: s.editing.name, oninput: (v) => (s.editing!.name = v) }),
        m('.mb-3', [m('label.form-label', { for: 'e-bio' }, 'Bio'), m('textarea.form-control', { id: 'e-bio', rows: 4, value: s.editing.bio, oninput: (e: Event) => (s.editing!.bio = (e.target as HTMLTextAreaElement).value) })]),
        field({ id: 'e-web', label: 'Website', value: s.editing.website_url ?? '', oninput: (v) => (s.editing!.website_url = v || null) }),
        m('.d-flex.gap-2', [
          m('button.btn.btn-sm.btn-primary', { type: 'button', onclick: async () => {
            try { await collar.update('pen_names', p.id, { name: s.editing!.name, bio: s.editing!.bio, website_url: s.editing!.website_url }); s.editing = null; await load(); toast('Saved.', 'success'); } catch (e) { toast(errorText(e), 'danger'); }
          } }, 'Save'),
          m('button.btn.btn-sm.btn-outline-secondary', { type: 'button', onclick: () => (s.editing = null) }, 'Cancel'),
        ]),
      ] : [
        m('h2.h5', p.name), m('p.small.text-body-secondary', '/authors/' + p.slug), m('p.small', p.bio || '—'),
        m('button.btn.btn-sm.btn-outline-primary', { type: 'button', onclick: () => (s.editing = { ...p }) }, 'Edit'),
      ]))))),
      m('.card', m('form.card-body', {
        onsubmit: async (ev: Event) => {
          ev.preventDefault(); s.fields = {};
          try { await reader.post('/studio/pen-names', { slug: s.slug, name: s.name, bio: s.bio }); s.slug = s.name = s.bio = ''; await load(); toast('Pen name created.', 'success'); }
          catch (e) { if (e instanceof ApiError) s.fields = e.fields; toast(errorText(e), 'danger'); }
        },
      }, [
        m('h2.h5', 'New pen name'),
        m('.row', [
          m('.col-md-6', field({ id: 'p-name', label: 'Name', value: s.name, oninput: (v) => { s.name = v; s.slug = v.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, ''); }, error: s.fields.name })),
          m('.col-md-6', field({ id: 'p-slug', label: 'Address', value: s.slug, oninput: (v) => (s.slug = v), error: s.fields.slug, help: '/authors/' + (s.slug || '…') })),
        ]),
        m('.mb-3', [m('label.form-label', { for: 'p-bio' }, 'Bio'), m('textarea.form-control', { id: 'p-bio', rows: 3, value: s.bio, oninput: (e: Event) => (s.bio = (e.target as HTMLTextAreaElement).value) })]),
        m('button.btn.btn-primary', { type: 'submit' }, 'Create pen name'),
      ])),
    ],
  };
}

// ---- collections ----------------------------------------------------------------------

export function Collections(): m.Component {
  const s = {
    items: null as CollectionRow[] | null, pens: [] as PenNameRow[], stories: [] as WorkRow[], error: '',
    title: '', pen: '', editing: null as null | { c: CollectionRow; order: string[] },
  };
  const load = async () => {
    try {
      const [c, p, w] = await Promise.all([
        collar.list<CollectionRow>('collections', { sort: '-updated_at', perPage: 100 }),
        collar.list<PenNameRow>('pen_names', { perPage: 100 }),
        collar.list<WorkRow>('works', { filter: 'kind = "short_story"', sort: 'title', perPage: 200 }),
      ]);
      s.items = c.items; s.pens = p.items; s.stories = w.items; s.pen ||= p.items[0]?.id ?? '';
    } catch (e) { s.error = errorText(e); }
    m.redraw();
  };
  const edit = async (c: CollectionRow) => {
    const items = await collar.list<{ work_id: string; ordinal: number }>('collection_items', { filter: `collection_id = ${c.id}`, sort: 'ordinal', perPage: 200 });
    s.editing = { c: { ...c }, order: items.items.map((x) => x.work_id) };
    m.redraw();
  };
  const title = (wid: string) => s.stories.find((w) => w.id === wid)?.title ?? `#${wid}`;
  return {
    oninit: () => { setTitle('Collections'); void load(); },
    view: () => [
      studioHeader('Collections'),
      m('p.text-body-secondary', 'An anthology is an ordered list of independent short stories. Stories keep their own addresses.'),
      s.error ? alertBox('danger', s.error) : s.items === null ? spinner() : [
        s.items.length ? m('ul.list-group.mb-4', s.items.map((c) => m('li.list-group-item.d-flex.justify-content-between.align-items-center', [
          m('span', [c.title, m('span.badge.text-bg-light.border.ms-2', c.visibility)]),
          m('button.btn.btn-sm.btn-outline-primary', { type: 'button', onclick: () => edit(c) }, 'Edit'),
        ]))) : emptyState('collection', 'No collections yet', ''),
        s.editing ? m('.card.mb-4', m('.card-body', [
          m('h2.h5', ['Edit “', s.editing.c.title, '”']),
          field({ id: 'c-title', label: 'Title', value: s.editing.c.title, oninput: (v) => (s.editing!.c.title = v) }),
          m('.mb-3', [m('label.form-label', { for: 'c-desc' }, 'Description'), m('textarea.form-control', { id: 'c-desc', rows: 3, value: s.editing.c.description, oninput: (e: Event) => (s.editing!.c.description = (e.target as HTMLTextAreaElement).value) })]),
          m('.mb-3', [m('label.form-label', { for: 'c-vis' }, 'Visibility'), m('select.form-select', { id: 'c-vis', value: s.editing.c.visibility, onchange: (e: Event) => (s.editing!.c.visibility = (e.target as HTMLSelectElement).value) },
            [['draft', 'Draft'], ['listed', 'Listed'], ['unlisted', 'Unlisted']].map(([v, l]) => m('option', { value: v }, l)))]),
          m('h3.h6', 'Stories (in order)'),
          m('ol.list-group.list-group-numbered.mb-2', s.editing.order.map((wid, i) => m('li.list-group-item.d-flex.justify-content-between', [
            title(wid),
            m('span', [
              m('button.btn.btn-sm.btn-light', { type: 'button', 'aria-label': 'Move up', disabled: i === 0, onclick: () => { const o = s.editing!.order; [o[i - 1], o[i]] = [o[i]!, o[i - 1]!]; } }, icon('arrow-up')),
              m('button.btn.btn-sm.btn-light', { type: 'button', 'aria-label': 'Remove', onclick: () => s.editing!.order.splice(i, 1) }, icon('x-lg')),
            ]),
          ]))),
          m('select.form-select.form-select-sm.mb-3', { 'aria-label': 'Add a story', value: '', onchange: (e: Event) => { const v = (e.target as HTMLSelectElement).value; if (v) s.editing!.order.push(v); } },
            [m('option', { value: '' }, 'Add a story…'), ...s.stories.filter((w) => !s.editing!.order.includes(w.id)).map((w) => m('option', { value: w.id }, w.title))]),
          m('.d-flex.gap-2', [
            m('button.btn.btn-primary', { type: 'button', onclick: async () => {
              const e = s.editing!;
              try {
                await collar.update('collections', e.c.id, { title: e.c.title, description: e.c.description, visibility: e.c.visibility });
                await reader.put(`/studio/collections/${e.c.id}/items`, { work_ids: e.order });
                s.editing = null; await load(); toast('Collection saved.', 'success');
              } catch (err) { toast(errorText(err), 'danger'); }
            } }, 'Save collection'),
            m('button.btn.btn-outline-secondary', { type: 'button', onclick: () => (s.editing = null) }, 'Cancel'),
          ]),
        ])) : null,
        m('.card', m('form.card-body', {
          onsubmit: async (ev: Event) => {
            ev.preventDefault();
            try { await reader.post('/studio/collections', { pen_name_id: s.pen, title: s.title }); s.title = ''; await load(); } catch (e) { toast(errorText(e), 'danger'); }
          },
        }, [
          m('h2.h5', 'New collection'),
          m('.row.g-2', [
            m('.col-md-7', field({ id: 'n-title', label: 'Title', value: s.title, oninput: (v) => (s.title = v) })),
            m('.col-md-5.mb-3', [m('label.form-label', { for: 'n-pen' }, 'Pen name'), m('select.form-select', { id: 'n-pen', value: s.pen, onchange: (e: Event) => (s.pen = (e.target as HTMLSelectElement).value) },
              s.pens.map((p) => m('option', { value: p.id }, p.name)))]),
          ]),
          m('button.btn.btn-primary', { type: 'submit', disabled: !s.title.trim() || !s.pen }, 'Create'),
        ])),
      ],
    ],
  };
}

// ---- media ------------------------------------------------------------------------------

export function Media(): m.Component {
  const s = { items: null as AssetRow[] | null, error: '', busy: false, alt: '' };
  const load = async () => {
    try { s.items = (await collar.list<AssetRow>('assets', { sort: '-created_at', perPage: 100 })).items; } catch (e) { s.error = errorText(e); }
    m.redraw();
  };
  return {
    oninit: () => { setTitle('Media'); void load(); },
    view: () => [
      studioHeader('Media'),
      m('.card.mb-4', m('.card-body', [
        m('p.small.mb-2', 'JPEG, PNG or WebP up to 5 MB and 5000 px. Every image is decoded and re-encoded; metadata (including location) is removed.'),
        m('.row.g-2', [
          m('.col-md-6', field({ id: 'alt', label: 'Alt text (describe the image)', value: s.alt, oninput: (v) => (s.alt = v), maxlength: 300 })),
          m('.col-md-6.mb-3', [m('label.form-label', { for: 'file' }, 'Image'), m('input.form-control', { id: 'file', type: 'file', accept: 'image/jpeg,image/png,image/webp', disabled: s.busy,
            onchange: async (ev: Event) => {
              const f = (ev.target as HTMLInputElement).files?.[0];
              if (!f) return;
              s.busy = true; m.redraw();
              try { await uploadImage(f, s.alt); toast('Image ready.', 'success'); s.alt = ''; await load(); } catch (e) { toast(errorText(e), 'danger'); }
              finally { s.busy = false; (ev.target as HTMLInputElement).value = ''; m.redraw(); }
            } })]),
        ]),
        s.busy ? m('p.small', { role: 'status' }, 'Uploading and checking…') : null,
      ])),
      s.error ? alertBox('danger', s.error) : s.items === null ? spinner() : s.items.length === 0 ? emptyState('images', 'No images yet', '') :
        m('.row.row-cols-2.row-cols-md-4.g-3', s.items.map((a) => m('.col', m('.card.h-100', [
          a.state === 'ready' ? m('img.card-img-top', { src: `/api/v1/pawn/x/assets/${a.id}/card`, alt: a.alt_text, loading: 'lazy' }) : null,
          m('.card-body.small', [m('span.badge', { class: a.state === 'ready' ? 'text-bg-success' : a.state === 'quarantined' ? 'text-bg-danger' : 'text-bg-secondary' }, a.state),
            a.error_code ? m('div.text-danger', a.error_code) : null, a.width ? m('div', `${a.width}×${a.height}`) : null, m('div.text-body-secondary', formatDate(a.created_at))]),
        ])))),
    ],
  };
}


