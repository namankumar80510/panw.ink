// Accessible status messages (aria-live) rendered with Bootstrap toast styles.
import m from 'mithril';

interface Toast {
  id: number;
  text: string;
  kind: 'success' | 'danger' | 'info';
}

let next = 1;
const items: Toast[] = [];

export function toast(text: string, kind: Toast['kind'] = 'info', ms = 5000): void {
  const t = { id: next++, text, kind };
  items.push(t);
  m.redraw();
  window.setTimeout(() => dismiss(t.id), ms);
}

function dismiss(id: number): void {
  const i = items.findIndex((t) => t.id === id);
  if (i >= 0) {
    items.splice(i, 1);
    m.redraw();
  }
}

export const Toasts: m.Component = {
  view: () =>
    m('.toast-container.position-fixed.bottom-0.end-0.p-3', { 'aria-live': 'polite', 'aria-atomic': 'false', role: 'status' },
      items.map((t) =>
        m('.toast.show.align-items-center.border-0', { key: t.id, class: `text-bg-${t.kind}` },
          m('.d-flex', [
            m('.toast-body', t.text),
            m('button.btn-close.btn-close-white.me-2.m-auto', { type: 'button', 'aria-label': 'Dismiss', onclick: () => dismiss(t.id) }),
          ]),
        ),
      ),
    ),
};
