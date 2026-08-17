import { EditorView } from '@codemirror/view'
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { tags } from '@lezer/highlight'
import type { Extension } from '@codemirror/state'

/**
 * 随主题自动适配的 CodeMirror 主题。
 * 所有颜色引用 Vuetify 的 `--v-theme-code-*` CSS 变量——切换深浅色时变量变化，
 * CodeMirror 通过 var() 自动跟随，无需 JS 监听或重建编辑器。
 * 用法：`cmExtensions = [basicSetup, json(), ...cmCodeMirrorTheme()]`
 */
export function cmCodeMirrorTheme(): Extension[] {
  return [
    EditorView.theme({
      '&': {
        backgroundColor: 'rgb(var(--v-theme-code-bg))',
        color: 'rgb(var(--v-theme-code-fg))',
      },
      '&.cm-focused': { outline: 'none' },
      '.cm-content': {
        fontFamily: "'JetBrains Mono', 'Fira Code', 'Consolas', 'Monaco', monospace",
        fontSize: '13px',
      },
      '.cm-scroller': {
        fontFamily: "'JetBrains Mono', 'Fira Code', 'Consolas', 'Monaco', monospace",
        fontSize: '13px',
      },
      '.cm-gutters': {
        backgroundColor: 'rgb(var(--v-theme-code-bg))',
        color: 'rgba(var(--v-theme-code-fg), 0.55)',
        borderRight: '1px solid rgba(var(--v-theme-code-fg), 0.18)',
      },
      '.cm-activeLine': { backgroundColor: 'rgba(var(--v-theme-code-fg), 0.06)' },
      '.cm-activeLineGutter': {
        backgroundColor: 'rgba(var(--v-theme-code-fg), 0.08)',
        color: 'rgb(var(--v-theme-code-fg))',
      },
      '.cm-selectionBackground, &.cm-focused .cm-selectionBackground': {
        backgroundColor: 'rgba(var(--v-theme-primary), 0.28)',
      },
      '.cm-cursor': { borderLeftColor: 'rgb(var(--v-theme-primary))' },
      '.cm-placeholder': { color: 'rgba(var(--v-theme-code-fg), 0.45)' },
      '&.cm-focused .cm-selectionBackground': { backgroundColor: 'rgba(var(--v-theme-primary), 0.28)' },
    }),
    syntaxHighlighting(
      HighlightStyle.define([
        { tag: tags.propertyName, color: 'rgb(var(--v-theme-code-property))' },
        { tag: tags.string, color: 'rgb(var(--v-theme-code-string))' },
        { tag: [tags.number, tags.bool, tags.null, tags.atom], color: 'rgb(var(--v-theme-code-number))' },
        { tag: tags.comment, color: 'rgb(var(--v-theme-code-comment))', fontStyle: 'italic' },
      ]),
    ),
  ]
}
