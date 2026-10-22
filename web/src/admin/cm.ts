// CodeMirror 6 Markdown editor, loaded lazily (its own Parcel bundle).
import { EditorState } from '@codemirror/state';
import { EditorView, keymap, placeholder } from '@codemirror/view';
import { defaultKeymap, history, historyKeymap } from '@codemirror/commands';
import { markdown } from '@codemirror/lang-markdown';
import { syntaxHighlighting, defaultHighlightStyle } from '@codemirror/language';
import { htmlToMarkdown } from './paste';

export interface EditorHandle {
  value(): string;
  set(text: string): void;
  focus(): void;
  destroy(): void;
}

export function createEditor(parent: HTMLElement, doc: string, onChange: (text: string) => void, onSave: () => void, label: string): EditorHandle {
  const view = new EditorView({
    parent,
    state: EditorState.create({
      doc,
      extensions: [
        history(),
        markdown(),
        syntaxHighlighting(defaultHighlightStyle),
        EditorView.lineWrapping,
        placeholder('Write in Markdown: *emphasis*, **strong**, > quotes, --- scene breaks…'),
        EditorView.contentAttributes.of({ 'aria-label': label, 'aria-multiline': 'true', spellcheck: 'true' }),
        keymap.of([{ key: 'Mod-s', preventDefault: true, run: () => { onSave(); return true; } }, ...defaultKeymap, ...historyKeymap]),
        EditorView.updateListener.of((u) => { if (u.docChanged) onChange(u.state.doc.toString()); }),
        // Rich text from Word/Google Docs becomes plain Markdown; pasted HTML is never inserted.
        EditorView.domEventHandlers({
          paste: (ev, v) => {
            const html = ev.clipboardData?.getData('text/html');
            if (!html) return false;
            ev.preventDefault();
            v.dispatch(v.state.replaceSelection(htmlToMarkdown(html)));
            return true;
          },
        }),
      ],
    }),
  });
  return {
    value: () => view.state.doc.toString(),
    set: (text) => view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: text } }),
    focus: () => view.focus(),
    destroy: () => view.destroy(),
  };
}
