// Entry editor (§11.1 steps 5–6, §11.2, §12.2): Markdown is the source of
// truth. Autosave after 1.5 s idle, manual save (Ctrl/Cmd-S), visible dirty
// state and version, 409 conflict recovery, a local unsaved copy that is
// only restored on explicit request, sanitized preview and revision history.
import m from 'mithril';
import { collar } from '../api/collar';
import { ApiError } from '../api/http';
import { reader } from '../api/reader';
import { alertBox, errorText, icon, spinner } from '../components/ui';
import { toast } from '../components/toasts';
import { formatCount, formatDate } from '../utils/format';
import { readJSON, remove, writeJSON } from '../utils/storage';
import { setTitle } from '../utils/title';
import { modal, modalController, releaseBadge, studioHeader } from './common';
import type { EditorHandle } from './cm';

interface Draft {
  entry_id: string; work_id: string; work_kind: 'novel' | 'short_story'; work_slug: string; work_title: string;
  title: string; slug: string; ordinal: number; workflow: 'draft' | 'scheduled' | 'published' | 'archived'; access: string;
  early_at: string | null; public_at: string | null; markdown: string; version: number; revision_id: string; word_count: number;
  saved_at: string; published: { id: string; version: number } | null; draft_is_published: boolean;
}
interface RevisionRow { id: string; version: number; word_count: number; change_note: string | null; created_at: string }
interface LocalCopy { base: number; text: string; at: string }

const AUTOSAVE_MS = 1500;
const localKey = (id: string) => 'pawn.draft.' + id;

function countWords(s: string): number {
  return (s.match(/[\p{L}\p{N}]+(?:['’.\-_][\p{L}\p{N}]+)*/gu) ?? []).length;
}

export function Editor(): m.Component {
  const id = m.route.param('id');
  let editor: EditorHandle | null = null;
  let timer = 0;
  let alive = true;
  const conflictModal = modalController();
  const s = {
    d: null as Draft | null, error: '', saving: false, dirty: false, text: '', base: 0,
    tab: 'write' as 'write' | 'preview' | 'history', previewHTML: '', revisions: null as RevisionRow[] | null,
    recovery: null as LocalCopy | null, conflict: null as null | { server: Draft }, title: '',
  };

  const persistLocal = () => writeJSON(localKey(id), { base: s.base, text: s.text, at: new Date().toISOString() } satisfies LocalCopy);

  const save = async (opts: { expected?: number } = {}) => {
    if (!alive || s.saving || !s.d) return;
    window.clearTimeout(timer);
    const text = s.text;
    s.saving = true; m.redraw();
    try {
      const d = await reader.put<Draft>(`/studio/entries/${id}/draft`, { markdown: text, expected_revision: opts.expected ?? s.base });
      if (!alive) return;
      s.d = { ...d, markdown: text };
      s.base = d.version;
      s.dirty = s.text !== text; // keep dirty if the author typed during the save
      if (!s.dirty) remove(localKey(id));
    } catch (e) {
      if (e instanceof ApiError && e.code === 'REVISION_CONFLICT') {
        const server = await reader.get<Draft>(`/studio/entries/${id}/draft`);
        s.conflict = { server };
        conflictModal.show();
      } else if (e instanceof ApiError && (e.status === 401 || e.status === 403 || e.status === 404)) {
        // Lost permission or signed out: stop autosaving; the local copy stays.
        alive = false;
        s.error = 'Saving stopped: ' + e.message + ' Your text is kept in this browser.';
      } else {
        toast('Could not save: ' + errorText(e), 'danger');
      }
    } finally { s.saving = false; m.redraw(); }
  };

  const onChange = (text: string) => {
    s.text = text;
    s.dirty = true;
    persistLocal();
    window.clearTimeout(timer);
    timer = window.setTimeout(() => void save(), AUTOSAVE_MS);
    m.redraw();
  };

  const loadPreview = async () => {
    try { s.previewHTML = (await reader.post<{ html: string }>(`/studio/entries/${id}/preview`, { markdown: s.text })).html; } catch (e) { s.previewHTML = ''; toast(errorText(e), 'danger'); }
    m.redraw();
  };
  const loadHistory = async () => {
    try { s.revisions = (await collar.list<RevisionRow>('studio_revisions', { filter: `entry_id = ${id}`, sort: '-version', perPage: 100 })).items; } catch (e) { toast(errorText(e), 'danger'); }
    m.redraw();
  };

  const beforeUnload = (ev: BeforeUnloadEvent) => { if (s.dirty) { ev.preventDefault(); } };

  return {
    oninit: async () => {
      try {
        const d = await reader.get<Draft>(`/studio/entries/${id}/draft`);
        s.d = d; s.text = d.markdown; s.base = d.version; s.title = d.title;
        setTitle(`Edit · ${d.title}`);
        const local = readJSON<LocalCopy | null>(localKey(id), null);
        if (local && local.text !== d.markdown) s.recovery = local; // offered, never applied silently
      } catch (e) { s.error = errorText(e); }
      m.redraw();
      window.addEventListener('beforeunload', beforeUnload);
    },
    onremove: () => {
      alive = false; // no autosave after leaving the page
      window.clearTimeout(timer);
      window.removeEventListener('beforeunload', beforeUnload);
      editor?.destroy();
    },
    view: () => {
      if (s.error && !s.d) return alertBox('danger', s.error);
      if (!s.d) return spinner();
      const d = s.d;
      const status = s.saving ? 'Saving…' : s.dirty ? 'Unsaved changes' : `Saved · version ${d.version} · ${formatDate(d.saved_at, true)}`;
      return [
        studioHeader(d.work_kind === 'novel' ? `Chapter ${d.ordinal}` : 'Story text', [
          m(m.route.Link, { href: `/studio/entries/${id}/schedule`, class: 'btn btn-outline-primary' }, [icon('calendar-event'), ' Release']),
        ], [['/studio/works', 'Works'], [`/studio/works/${d.work_id}/entries`, d.work_title]]),
        s.error ? alertBox('warning', s.error) : null,
        s.recovery ? m('.alert.alert-info.d-flex.flex-wrap.align-items-center.gap-2', { role: 'status' }, [
          m('span.me-auto', `This browser has unsaved text from ${formatDate(s.recovery.at, true)} (based on version ${s.recovery.base}).`),
          m('button.btn.btn-sm.btn-primary', { type: 'button', onclick: () => { editor?.set(s.recovery!.text); s.recovery = null; } }, 'Restore my unsaved text'),
          m('button.btn.btn-sm.btn-outline-secondary', { type: 'button', onclick: () => { remove(localKey(id)); s.recovery = null; } }, 'Discard it'),
        ]) : null,
        m('.row.g-2.align-items-end.mb-3', [
          m('.col-md-8', [m('label.form-label.small', { for: 'entry-title' }, 'Title'),
            m('input.form-control', { id: 'entry-title', value: s.title, maxlength: 240, oninput: (e: Event) => (s.title = (e.target as HTMLInputElement).value),
              onblur: async () => {
                if (s.title.trim() && s.title !== d.title) {
                  try { await collar.update('entries', id, { title: s.title.trim() }); d.title = s.title.trim(); toast('Title saved.', 'success'); } catch (e) { toast(errorText(e), 'danger'); }
                }
              } })]),
          m('.col-md-4.text-md-end', [releaseBadge({ workflow: d.workflow, early_at: d.early_at, public_at: d.public_at, published_revision_id: d.published?.id ?? null }),
            d.published && !d.draft_is_published ? m('div.small.text-warning-emphasis.mt-1', 'Draft differs from the published text.') : null]),
        ]),
        m('ul.nav.nav-tabs', { role: 'tablist' }, ([['write', 'Write'], ['preview', 'Preview'], ['history', 'History']] as const).map(([k, l]) =>
          m('li.nav-item', { role: 'presentation' }, m('button.nav-link', {
            type: 'button', role: 'tab', class: s.tab === k ? 'active' : '', 'aria-selected': s.tab === k ? 'true' : 'false',
            onclick: () => { s.tab = k; if (k === 'preview') void loadPreview(); if (k === 'history') void loadHistory(); },
          }, l)))),
        m('.border.border-top-0.rounded-bottom.p-0.mb-2', [
          m('div', {
            class: 'studio-editor', style: { display: s.tab === 'write' ? 'block' : 'none' },
            oncreate: async (v: m.VnodeDOM) => {
              const { createEditor } = await import('./cm');
              if (!alive) return;
              editor = createEditor(v.dom as HTMLElement, s.text, onChange, () => void save(), `Text of ${d.title}`);
              editor.focus();
            },
          }),
          s.tab === 'preview' ? m('.p-4', m('article.reader-text', [m('h1.rail-title', s.title), m('.entry-body', m.trust(s.previewHTML))])) : null,
          s.tab === 'history' ? m('.p-3', s.revisions === null ? spinner() : m('table.table.table-sm', [
            m('thead', m('tr', ['Version', 'Saved', 'Words', 'Note', ''].map((h) => m('th', h)))),
            m('tbody', s.revisions.map((r) => m('tr', [
              m('td', ['v' + r.version, r.version === d.version ? m('span.badge.text-bg-primary.ms-1', 'draft') : null, d.published && r.id === d.published.id ? m('span.badge.text-bg-success.ms-1', 'published') : null]),
              m('td.small', formatDate(r.created_at, true)), m('td', formatCount(r.word_count)), m('td.small', r.change_note ?? ''),
              m('td', r.version === d.version ? null : m('button.btn.btn-sm.btn-outline-secondary', { type: 'button', onclick: async () => {
                try {
                  const nd = await reader.post<Draft>(`/studio/entries/${id}/restore`, { revision_id: r.id });
                  s.d = nd; s.base = nd.version; s.text = nd.markdown; editor?.set(nd.markdown); s.dirty = false; remove(localKey(id));
                  toast(`Restored version ${r.version} as version ${nd.version}.`, 'success'); void loadHistory();
                } catch (e) { toast(errorText(e), 'danger'); }
              } }, 'Restore')),
            ]))),
          ])) : null,
        ]),
        m('.d-flex.flex-wrap.align-items-center.gap-3', [
          m('button.btn.btn-primary', { type: 'button', disabled: s.saving || !s.dirty, onclick: () => save() }, 'Save'),
          m('span.small', { role: 'status', 'aria-live': 'polite', class: s.dirty ? 'text-warning-emphasis' : 'text-body-secondary' }, status),
          m('span.small.text-body-secondary.ms-auto', `${formatCount(countWords(s.text))} words · ~${Math.max(1, Math.ceil(countWords(s.text) / 230))} min`),
        ]),
        modal(conflictModal, 'conflict', 'This text changed elsewhere', s.conflict ? [
          m('p', `Another tab or device saved version ${s.conflict.server.version} at ${formatDate(s.conflict.server.saved_at, true)}. Your text was not saved. Choose which version to keep — the other remains in the history.`),
          m('details', [m('summary', 'Show the saved version'), m('pre.small.border.rounded.p-2.mt-2', { style: { maxHeight: '30vh', whiteSpace: 'pre-wrap' } }, s.conflict.server.markdown)]),
        ] : null, [
          m('button.btn.btn-outline-secondary', { type: 'button', onclick: () => {
            const srv = s.conflict!.server; s.conflict = null; conflictModal.hide();
            persistLocal(); // my text stays recoverable in this browser
            s.d = srv; s.base = srv.version; s.text = srv.markdown; editor?.set(srv.markdown); s.dirty = false;
          } }, 'Use the saved version'),
          m('button.btn.btn-primary', { type: 'button', onclick: () => {
            const v = s.conflict!.server.version; s.conflict = null; conflictModal.hide(); void save({ expected: v });
          } }, 'Keep my version'),
        ]),
      ];
    },
  };
}
