// Small shared view helpers built from Bootstrap components.
import m from 'mithril';
import { ApiError } from '../api/http';

export function icon(name: string, label?: string): m.Vnode {
  return label
    ? m(`i.bi.bi-${name}`, { role: 'img', 'aria-label': label })
    : m(`i.bi.bi-${name}`, { 'aria-hidden': 'true' });
}

export function emptyState(iconName: string, title: string, body: m.Children): m.Vnode {
  return m('.empty-state', [icon(iconName), m('p.fw-semibold.mb-1', title), m('div.small', body)]);
}

export function errorText(e: unknown): string {
  if (e instanceof ApiError) return e.message;
  return 'Something went wrong. Please try again.';
}

export function alertBox(kind: 'danger' | 'success' | 'info' | 'warning', text: m.Children): m.Vnode {
  return m(`.alert.alert-${kind}`, { role: kind === 'danger' ? 'alert' : 'status' }, text);
}

export interface FieldAttrs {
  id: string;
  label: string;
  type?: string;
  value: string;
  oninput: (v: string) => void;
  error?: string | undefined;
  help?: string;
  autocomplete?: string;
  required?: boolean;
  minlength?: number;
  maxlength?: number;
  disabled?: boolean;
}

/** A labelled Bootstrap form field with accessible error/help text. */
export function field(a: FieldAttrs): m.Vnode {
  const describedBy = [a.help ? a.id + '-help' : '', a.error ? a.id + '-error' : ''].filter(Boolean).join(' ') || undefined;
  return m('.mb-3', [
    m('label.form-label', { for: a.id }, a.label),
    m('input.form-control', {
      id: a.id,
      name: a.id,
      type: a.type ?? 'text',
      value: a.value,
      class: a.error ? 'is-invalid' : '',
      autocomplete: a.autocomplete,
      required: a.required,
      minlength: a.minlength,
      maxlength: a.maxlength,
      disabled: a.disabled,
      'aria-invalid': a.error ? 'true' : undefined,
      'aria-describedby': describedBy,
      oninput: (e: InputEvent) => a.oninput((e.target as HTMLInputElement).value),
    }),
    a.help ? m('.form-text', { id: a.id + '-help' }, a.help) : null,
    a.error ? m('.invalid-feedback', { id: a.id + '-error' }, a.error) : null,
  ]);
}

export function submitButton(label: string, busy: boolean, cls = 'btn-primary'): m.Vnode {
  return m(`button.btn.${cls}`, { type: 'submit', disabled: busy, 'aria-busy': busy ? 'true' : undefined }, [
    busy ? m('span.spinner-border.spinner-border-sm.me-2', { 'aria-hidden': 'true' }) : null,
    label,
  ]);
}

export function pageHeader(title: string, lead?: m.Children): m.Vnode {
  return m('header.mb-4', [m('h1.h2.rail-title', title), lead ? m('p.lead.text-body-secondary', lead) : null]);
}

export function spinner(label = 'Loading'): m.Vnode {
  return m('.d-flex.justify-content-center.py-5', m('.spinner-border', { role: 'status' }, m('span.visually-hidden', label)));
}
