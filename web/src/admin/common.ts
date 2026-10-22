// Shared studio helpers: native record types, release-status badges, a
// Bootstrap modal with lifecycle-managed instances, and the upload pipeline.
import m from 'mithril';
import Modal from 'bootstrap/js/dist/modal';
import { ApiError, readJSON, send } from '../api/http';
import { reader } from '../api/reader';
import { icon } from '../components/ui';
import { session } from '../stores/session';
import { formatDate } from '../utils/format';

/** Native Collar rows (bigintAsString: ids are strings on output; send numbers back). */
export interface PenNameRow { id: string; slug: string; name: string; bio: string; website_url: string | null; avatar_asset_id: string | null; state: string }
export interface WorkRow {
  id: string; pen_name_id: string; kind: 'novel' | 'short_story'; slug: string; title: string; subtitle: string | null; synopsis: string;
  cover_asset_id: string | null; primary_genre_id: string; publication_status: string; visibility: string; age_rating: string;
  language_code: string; content_warnings_json: string[] | null; first_public_at: string | null; last_public_update_at: string | null;
  completed_at: string | null; public_entry_count: number; public_word_count: number; updated_at: string;
  expand?: { pen_name_id?: PenNameRow };
}
export interface StudioEntryRow {
  id: string; work_id: string; work_kind: string; volume_id: string | null; ordinal: number; slug: string; title: string;
  workflow: 'draft' | 'scheduled' | 'published' | 'archived'; early_at: string | null; public_at: string | null;
  draft_version: number | null; draft_word_count: number | null; draft_saved_at: string | null;
  published_version: number | null; published_word_count: number | null; minimum_access_rank: number | null;
  current_draft_revision_id: string | null; published_revision_id: string | null;
}
export interface VolumeRow { id: string; work_id: string; number: number; title: string }
export interface AssetRow { id: string; state: string; error_code: string | null; alt_text: string; width: number | null; height: number | null; created_at: string }
export interface CollectionRow { id: string; pen_name_id: string; slug: string; title: string; description: string; visibility: string; cover_asset_id: string | null }

export type Release = 'draft' | 'archived' | 'upcoming' | 'early' | 'public';

/** Release state from dates, exactly as the server computes it. */
export function releaseOf(e: Pick<StudioEntryRow, 'workflow' | 'early_at' | 'public_at' | 'published_revision_id'>, now = Date.now()): Release {
  if (e.workflow === 'draft' || e.workflow === 'archived' || !e.published_revision_id || !e.public_at) return e.workflow === 'archived' ? 'archived' : 'draft';
  if (now >= Date.parse(e.public_at)) return 'public';
  if (e.early_at && now >= Date.parse(e.early_at)) return 'early';
  return 'upcoming';
}

export function releaseBadge(e: Pick<StudioEntryRow, 'workflow' | 'early_at' | 'public_at' | 'published_revision_id'>): m.Children {
  const r = releaseOf(e);
  const map: Record<Release, [string, string]> = {
    draft: ['text-bg-secondary', 'Draft'],
    archived: ['text-bg-dark', 'Unpublished'],
    upcoming: ['text-bg-light border', 'Scheduled · free ' + formatDate(e.public_at, true)],
    early: ['text-bg-info', 'Early access · free ' + formatDate(e.public_at, true)],
    public: ['text-bg-success', 'Public since ' + formatDate(e.public_at)],
  };
  const [cls, label] = map[r];
  return m('span.badge', { class: cls }, label);
}

export function studioHeader(title: string, actions?: m.Children, crumbs?: Array<[string, string]>): m.Vnode {
  return m('header.mb-4', [
    crumbs ? m('nav', { 'aria-label': 'Breadcrumb' }, m('ol.breadcrumb.small.mb-1', crumbs.map(([href, label]) => m('li.breadcrumb-item', m(m.route.Link, { href }, label))))) : null,
    m('.d-flex.flex-wrap.justify-content-between.align-items-center.gap-2', [m('h1.h3.mb-0', title), actions ? m('.d-flex.flex-wrap.gap-2', actions) : null]),
  ]);
}

/** A Bootstrap modal whose instance lives exactly as long as the vnode. */
export function modalController() {
  let inst: Modal | null = null;
  return {
    attrs: {
      oncreate: (v: m.VnodeDOM) => { inst = Modal.getOrCreateInstance(v.dom as HTMLElement); },
      onremove: () => { inst?.dispose(); inst = null; },
    },
    show: () => inst?.show(),
    hide: () => inst?.hide(),
  };
}

export function modal(ctl: ReturnType<typeof modalController>, id: string, title: string, body: m.Children, footer: m.Children): m.Vnode {
  return m('.modal.fade', { id, tabindex: -1, 'aria-labelledby': id + '-title', 'aria-hidden': 'true', ...ctl.attrs },
    m('.modal-dialog.modal-dialog-scrollable', m('.modal-content', [
      m('.modal-header', [m('h2.modal-title.h5', { id: id + '-title' }, title), m('button.btn-close', { type: 'button', 'aria-label': 'Close', onclick: ctl.hide })]),
      m('.modal-body', body),
      m('.modal-footer', footer),
    ])));
}

export const toNum = (id: string | null | undefined): number | null => (id ? Number(id) : null);

/**
 * Uploads an image: Collar presign -> direct PUT to the bucket -> Collar
 * confirm -> Pawn.Ink registration, which validates and re-encodes it.
 */
export async function uploadImage(file: File, altText: string): Promise<{ id: string; state: string; card_url: string | null }> {
  if (!['image/jpeg', 'image/png', 'image/webp'].includes(file.type)) throw new ApiError(422, 'TYPE', 'Choose a JPEG, PNG or WebP image.');
  if (file.size > 5 * 1024 * 1024) throw new ApiError(422, 'SIZE', 'Images must be 5 MB or smaller.');
  const presignRes = await send('/api/v1/pawn/storage/uploads/presign', { method: 'POST', body: { filename: file.name, contentType: file.type, size: file.size } });
  const presign = (await readJSON(presignRes)) as { key: string; url: string; headers: Record<string, string>; uploadToken: string; error?: { message: string } };
  if (!presignRes.ok) throw new ApiError(presignRes.status, 'PRESIGN', presign.error?.message ?? 'Upload could not start.');
  const put = await fetch(presign.url, { method: 'PUT', headers: presign.headers, body: file });
  if (!put.ok) throw new ApiError(put.status, 'UPLOAD', 'The upload failed. Please try again.');
  const confirmRes = await send('/api/v1/pawn/storage/uploads/confirm', { method: 'POST', body: { uploadToken: presign.uploadToken } });
  if (!confirmRes.ok) throw new ApiError(confirmRes.status, 'CONFIRM', 'The upload was rejected (size or type).');
  return reader.post('/studio/assets', { key: presign.key, alt_text: altText });
}

export function needsVerification(): m.Children {
  return session.user && !session.user.email_verified
    ? m('.alert.alert-warning', [icon('envelope-exclamation'), ' Verify your email address to use the studio.'])
    : null;
}
