<script>
  import { onMount } from 'svelte';
  import { basicSetup } from 'codemirror';
  import { Compartment, EditorState } from '@codemirror/state';
  import { EditorView } from '@codemirror/view';
  import { json } from '@codemirror/lang-json';
  import { linter, lintGutter } from '@codemirror/lint';
  import { inspectJSON, position } from './lib/json.js';

  let { value = $bindable(''), id, label, kind = 'json', readOnly = false, disabled = false, onchange = () => {} } = $props();
  let host;
  let view = $state.raw(null);
  let replacing = false;
  const editable = new Compartment();
  const analysis = $derived(inspectJSON(value, kind));

  onMount(() => {
    view = new EditorView({
      doc: value,
      parent: host,
      extensions: [
        basicSetup, json(), EditorView.lineWrapping, lintGutter(),
        linter((editor) => inspectJSON(editor.state.doc.toString(), kind).issues, { delay: 200 }),
        editable.of([EditorState.readOnly.of(readOnly || disabled), EditorView.editable.of(!readOnly && !disabled)]),
        EditorView.contentAttributes.of({ id, 'aria-label': label, 'aria-multiline': 'true', 'aria-describedby': id + '-status', spellcheck: 'false' }),
        EditorView.updateListener.of((update) => {
          if (update.docChanged && !replacing) {
            value = update.state.doc.toString();
            onchange();
          }
        }),
      ],
    });
    return () => view?.destroy();
  });

  $effect(() => {
    if (view && view.state.doc.toString() !== value) {
      replacing = true;
      try { view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: value } }); }
      finally { replacing = false; }
    }
  });
  $effect(() => {
    view?.dispatch({ effects: editable.reconfigure([EditorState.readOnly.of(readOnly || disabled), EditorView.editable.of(!readOnly && !disabled)]) });
  });

  function format() {
    if (!analysis.valid || disabled) return;
    value = JSON.stringify(analysis.value, null, 2);
  }
  function jump(issue) {
    view?.dispatch({ selection: { anchor: issue.from }, scrollIntoView: true });
    view?.focus();
  }
</script>

<div class="json-field" class:read-only={readOnly} class:has-errors={!analysis.valid}>
  <div class="label-row editor-label">
    <label for={id}>{label} <span class="json-tag">JSON</span></label>
    {#if !readOnly}<button type="button" class="format-json" onclick={format} disabled={disabled || !analysis.valid}>Format JSON</button>{/if}
  </div>
  <div class="code-editor" bind:this={host}></div>
  <div id={id + '-status'} class="lint-status" class:invalid={!analysis.valid} aria-live="polite">
    {#if analysis.valid}
      <span>✓ Valid JSON{kind === 'questions' ? ' · ' + Object.keys(analysis.value).length + ' questions' : ''}</span>
      <span>{value.length.toLocaleString()} characters</span>
    {:else}
      <span>{analysis.issues.length} {analysis.issues.length === 1 ? 'issue' : 'issues'} to fix</span>
    {/if}
  </div>
  {#if !analysis.valid}
    <ul class="lint-errors">
      {#each analysis.issues.slice(0, 3) as issue}
        <li><button type="button" onclick={() => jump(issue)}>{position(value, issue.from)}: {issue.message}</button></li>
      {/each}
    </ul>
  {/if}
</div>
