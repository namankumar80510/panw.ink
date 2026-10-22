// Release screen (§11.1 steps 7–9, §11.2): public date (required), optional
// earlier supporter access with a tier rank, time-zone aware input with a UTC
// preview, publish now, publish corrections, and audited unpublishing.
import m from 'mithril';
import { reader } from '../api/reader';
import { alertBox, errorText, field, icon, spinner } from '../components/ui';
import { toast } from '../components/toasts';
import { config } from '../stores/config';
import { session } from '../stores/session';
import { formatDate } from '../utils/format';
import { setTitle } from '../utils/title';
import { browserZone, utcToWall, wallToUTC } from '../utils/tz';
import { modal, modalController, releaseBadge, releaseOf, studioHeader } from './common';

interface Draft {
  entry_id: string; work_id: string; work_title: string; work_kind: string; work_slug: string; title: string; slug: string; ordinal: number;
  workflow: 'draft' | 'scheduled' | 'published' | 'archived'; early_at: string | null; public_at: string | null; min_access_rank: number | null;
  version: number; word_count: number; published: { id: string; version: number } | null; draft_is_published: boolean;
}

export function Schedule(): m.Component {
  const id = m.route.param('id');
  const confirmNow = modalController();
  const confirmUnpublish = modalController();
  const s = {
    d: null as Draft | null, error: '', zone: browserZone(), pub: '', earlyOn: false, early: '', rank: 1, busy: false,
    fields: {} as Record<string, string>, reason: '',
  };
  const load = async () => {
    try {
      const d = await reader.get<Draft>(`/studio/entries/${id}/draft`);
      s.d = d;
      s.zone = session.user?.timezone || browserZone();
      if (d.public_at) s.pub = utcToWall(new Date(d.public_at), s.zone);
      s.earlyOn = !!d.early_at;
      if (d.early_at) s.early = utcToWall(new Date(d.early_at), s.zone);
      s.rank = d.min_access_rank ?? 1;
      setTitle(`Release · ${d.title}`);
    } catch (e) { s.error = errorText(e); }
    m.redraw();
  };
  const utc = (wall: string) => wallToUTC(wall, s.zone);
  const call = async (path: string, body: unknown, msg: string) => {
    s.busy = true; s.fields = {};
    try {
      await reader.post(`/studio/entries/${id}/${path}`, body);
      toast(msg, 'success');
      await load();
    } catch (e) {
      const f = (e as { fields?: Record<string, string> }).fields;
      if (f && Object.keys(f).length) s.fields = f;
      toast(errorText(e), 'danger');
    } finally { s.busy = false; m.redraw(); }
  };
  return {
    oninit: load,
    view: () => {
      if (s.error) return alertBox('danger', s.error);
      if (!s.d) return spinner();
      const d = s.d;
      const state = releaseOf({ workflow: d.workflow, early_at: d.early_at, public_at: d.public_at, published_revision_id: d.published?.id ?? null });
      const pubUTC = utc(s.pub);
      const earlyUTC = s.earlyOn ? utc(s.early) : null;
      const maxDays = config.value?.limits.max_early_days ?? 30;
      const leadDays = pubUTC && earlyUTC ? (pubUTC.getTime() - earlyUTC.getTime()) / 86_400_000 : 0;
      const label = d.work_kind === 'novel' ? 'chapter' : 'story';
      return [
        studioHeader(`Release “${d.title}”`, m(m.route.Link, { href: `/studio/entries/${id}/edit`, class: 'btn btn-outline-secondary' }, [icon('pencil'), ' Edit text']),
          [['/studio/works', 'Works'], [`/studio/works/${d.work_id}/entries`, d.work_title]]),
        m('p', ['Current status: ', releaseBadge({ workflow: d.workflow, early_at: d.early_at, public_at: d.public_at, published_revision_id: d.published?.id ?? null })]),
        d.word_count === 0 ? alertBox('warning', ['The draft is empty. ', m(m.route.Link, { href: `/studio/entries/${id}/edit` }, 'Write the text'), ' before releasing it.']) : null,

        state === 'public' ? m('.card.mb-4', m('.card-body', [
          m('h2.h5', 'Published'),
          m('p', [`This ${label} has been free for everyone since `, m('strong', formatDate(d.public_at, true)), '. Its public date cannot change.']),
          d.draft_is_published ? m('p.text-body-secondary', `The published text is the current draft (version ${d.version}).`) : [
            m('p', `The draft (version ${d.version}) differs from the published text (version ${d.published?.version}). Publishing the correction keeps the release date, so it does not count as a new update.`),
            m('button.btn.btn-primary', { type: 'button', disabled: s.busy, onclick: () => call('publish-revision', {}, 'Correction published.') }, 'Update published text'),
          ],
        ])) : m('form.card.mb-4', { novalidate: true, onsubmit: (e: Event) => {
          e.preventDefault();
          if (!pubUTC) { s.fields = { public_at: 'Choose the public date.' }; return; }
          void call('schedule', { public_at: pubUTC.toISOString(), early_at: earlyUTC ? earlyUTC.toISOString() : null, min_access_rank: s.earlyOn ? s.rank : null },
            `Scheduled: free for everyone on ${formatDate(pubUTC.toISOString(), true)}.`);
        } }, m('.card-body', [
          m('h2.h5', 'Schedule'),
          m('p.small.text-body-secondary', `Everything becomes free on its public date. Supporters may read up to ${maxDays} days earlier.`),
          m('.row.g-3', [
            m('.col-md-6', [m('label.form-label', { for: 'pub' }, 'Free for everyone on'),
              m('input.form-control', { id: 'pub', type: 'datetime-local', value: s.pub, class: s.fields.public_at ? 'is-invalid' : '',
                oninput: (e: Event) => (s.pub = (e.target as HTMLInputElement).value), 'aria-describedby': 'pub-help' }),
              s.fields.public_at ? m('.invalid-feedback', s.fields.public_at) : null,
              m('.form-text', { id: 'pub-help' }, pubUTC ? `= ${pubUTC.toISOString().replace('T', ' ').slice(0, 16)} UTC` : 'Required.')]),
            m('.col-md-6', [m('label.form-label', { for: 'zone' }, 'Time zone'),
              m('input.form-control', { id: 'zone', value: s.zone, list: 'zones', oninput: (e: Event) => (s.zone = (e.target as HTMLInputElement).value) }),
              m('datalist', { id: 'zones' }, ((Intl as unknown as { supportedValuesOf?: (k: string) => string[] }).supportedValuesOf?.('timeZone') ?? ['UTC']).map((z) => m('option', { value: z })))]),
          ]),
          m('.form-check.form-switch.my-3', [
            m('input.form-check-input', { type: 'checkbox', role: 'switch', id: 'early-on', checked: s.earlyOn, disabled: state === 'early', onchange: () => (s.earlyOn = !s.earlyOn) }),
            m('label.form-check-label', { for: 'early-on' }, 'Early access for Patreon supporters'),
          ]),
          s.earlyOn ? m('.row.g-3', [
            m('.col-md-6', [m('label.form-label', { for: 'early' }, 'Supporters can read from'),
              m('input.form-control', { id: 'early', type: 'datetime-local', value: s.early, disabled: state === 'early', class: s.fields.early_at ? 'is-invalid' : '',
                oninput: (e: Event) => (s.early = (e.target as HTMLInputElement).value) }),
              s.fields.early_at ? m('.invalid-feedback', s.fields.early_at) : null,
              m('.form-text', earlyUTC ? `= ${earlyUTC.toISOString().replace('T', ' ').slice(0, 16)} UTC · ${leadDays.toFixed(1)} days early` : ''),
              leadDays > maxDays ? m('.text-danger.small', `At most ${maxDays} days of early access.`) : null]),
            m('.col-md-6', [m('label.form-label', { for: 'rank' }, 'Minimum supporter tier rank'),
              m('input.form-control', { id: 'rank', type: 'number', min: 1, max: 100, value: s.rank, class: s.fields.min_access_rank ? 'is-invalid' : '', oninput: (e: Event) => (s.rank = Number((e.target as HTMLInputElement).value)) }),
              s.fields.min_access_rank ? m('.invalid-feedback', s.fields.min_access_rank) : null,
              m('.form-text', 'Ranks map to Patreon tiers in Integrations → Patreon. Higher ranks include lower ones.')]),
          ]) : null,
          state === 'early' ? alertBox('info', 'Early access has started: the public date can only move earlier, and the early start is fixed.') : null,
          m('.d-flex.flex-wrap.gap-2.mt-3', [
            m('button.btn.btn-primary', { type: 'submit', disabled: s.busy || d.word_count === 0 || leadDays > maxDays }, d.workflow === 'draft' || d.workflow === 'archived' ? 'Schedule release' : 'Update schedule'),
            m('button.btn.btn-outline-success', { type: 'button', disabled: s.busy || d.word_count === 0, onclick: () => confirmNow.show() }, 'Publish now…'),
          ]),
          m('p.small.text-body-secondary.mt-2.mb-0', 'Scheduling freezes the current draft as the published text. Later edits stay private until you publish them.'),
        ])),

        state !== 'draft' && state !== 'archived' ? m('.card.border-danger', m('.card-body', [
          m('h2.h6', 'Unpublish'),
          m('p.small', `Exceptional: withdraws this ${label} for rights or safety reasons. It is recorded in the audit log; the text, its address and readers' progress are kept.`),
          m('button.btn.btn-sm.btn-outline-danger', { type: 'button', onclick: () => confirmUnpublish.show() }, 'Unpublish…'),
        ])) : null,

        modal(confirmNow, 'confirm-now', 'Publish now?', m('p', `“${d.title}” becomes free for everyone immediately, with the current draft (version ${d.version}) as its text. This cannot be undone except by an audited unpublish.`), [
          m('button.btn.btn-outline-secondary', { type: 'button', onclick: confirmNow.hide }, 'Cancel'),
          m('button.btn.btn-success', { type: 'button', onclick: () => { confirmNow.hide(); void call('publish-now', {}, 'Published.'); } }, 'Publish now'),
        ]),
        modal(confirmUnpublish, 'confirm-unpublish', 'Unpublish?', [
          m('p', 'Readers will no longer be able to open this text. Give a reason for the audit log.'),
          field({ id: 'reason', label: 'Reason', value: s.reason, oninput: (v) => (s.reason = v) }),
        ], [
          m('button.btn.btn-outline-secondary', { type: 'button', onclick: confirmUnpublish.hide }, 'Cancel'),
          m('button.btn.btn-danger', { type: 'button', disabled: s.reason.trim().length < 3, onclick: () => { confirmUnpublish.hide(); void call('unpublish', { reason: s.reason }, 'Unpublished.'); } }, 'Unpublish'),
        ]),
      ];
    },
  };
}
