<script>
  import { onMount, tick } from 'svelte';
  import JsonEditor from './JsonEditor.svelte';
  import DecisionCard from './DecisionCard.svelte';
  import { inspectJSON } from './lib/json.js';

  let examples = $state({});
  let models = $state([]);
  let modelKey = $state('typed-decisions');
  let selectedExample = $state('refund');
  let query = $state('');
  let stateJSON = $state('{}');
  let questionsJSON = $state('{}');
  let apiOnline = $state(false);
  let examplesLoaded = $state(false);
  let busy = $state(false);
  let error = $state('');
  let result = $state(null);
  let lastRequest = $state(null);
  let showAPI = $state(false);
  let resultCard;
  const model = $derived(models.find((item) => item.key === modelKey));
  const ready = $derived(apiOnline && model?.status === 'ready');
  const stateCheck = $derived(inspectJSON(stateJSON, 'state'));
  const questionsCheck = $derived(inspectJSON(questionsJSON, 'questions'));
  const valid = $derived(stateCheck.valid && questionsCheck.valid);
  const canRun = $derived(ready && examplesLoaded && valid && !busy);
  const filteredExamples = $derived(Object.entries(examples).filter(([, example]) => (example.title + ' ' + example.category).toLowerCase().includes(query.trim().toLowerCase())));
  const connectionStatus = $derived(!apiOnline ? 'offline' : model?.status || 'loading');
  const connectionLabel = $derived(!apiOnline ? 'Connecting to local API' : ready ? 'Local model ready' : model?.status === 'error' ? 'Model unavailable' : 'Loading and warming…');

  function clearResult() { result = null; showAPI = false; error = ''; }
  function edited() { selectedExample = ''; clearResult(); }
  function loadExample(key) {
    if (busy) return;
    const example = examples[key];
    stateJSON = JSON.stringify(example.state, null, 2);
    questionsJSON = JSON.stringify(example.questions, null, 2);
    selectedExample = key;
    clearResult();
  }

  onMount(() => {
    let stopped = false;
    let timer;
    const controller = new AbortController();
    async function health() {
      try {
        const response = await fetch('/api/health', { signal: AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]), cache: 'no-store' });
        if (!response.ok) throw new Error('API unavailable');
        const data = await response.json();
        if (!stopped) { models = data.models; apiOnline = true; }
      } catch { if (!stopped) apiOnline = false; }
      finally { if (!stopped) timer = setTimeout(health, apiOnline && models.every((item) => item.status !== 'loading') ? 15000 : 2000); }
    }
    async function load() {
      try {
        const response = await fetch('/examples.json', { signal: controller.signal });
        if (!response.ok) throw new Error('Could not load the examples. Refresh the page.');
        const data = await response.json();
        if (!stopped) { examples = data; examplesLoaded = true; loadExample('refund'); }
      } catch (err) { if (!stopped) error = err.message; }
    }
    health();
    load();
    return () => { stopped = true; clearTimeout(timer); controller.abort(); };
  });

  async function run(event) {
    event?.preventDefault();
    if (!canRun) return;
    lastRequest = { model: modelKey, state: stateCheck.value, questions: questionsCheck.value };
    clearResult();
    busy = true;
    try {
      const response = await fetch('/api/predict', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(lastRequest), signal: AbortSignal.timeout(125000),
      });
      const data = await response.json();
      if (!response.ok) throw new Error(typeof data.detail === 'string' ? data.detail : 'The request could not be completed.');
      result = data;
      await tick();
      if (window.innerWidth <= 760) resultCard.scrollIntoView({ behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'instant' : 'smooth', block: 'start' });
    } catch (err) {
      error = err.name === 'TimeoutError' ? 'The model took too long. Check the local server.' : err instanceof TypeError ? 'Could not reach the local API. Check that the server is running.' : err.message;
    } finally { busy = false; }
  }
  function shortcut(event) {
    if ((event.ctrlKey || event.metaKey) && event.key === 'Enter') { event.preventDefault(); run(); }
  }
</script>

<svelte:window onkeydown={shortcut} />

<header class="topbar">
  <a class="brand" href="/" aria-label="Laya home"><img src="/icon.svg" width="34" height="34" alt="" /><span>laya<span class="brand-divider">/</span><span class="brand-caption">decision playground</span></span></a>
  <div class="connection" data-status={connectionStatus} role="status"><span class="status-dot"></span><span>{connectionLabel}</span></div>
</header>

<main>
  <div class="intro">
    <div><p class="eyebrow">ONE STATE. SEVERAL DECISIONS.</p><h1>Make the next call.</h1><p class="lede">Give Laya structured context and typed questions.<br />Explore yes/no decisions, choices, and scores in one pass.</p></div>
    <div class="model-note"><span class="model-note-label">RUNNING LOCALLY</span><a href={'https://huggingface.co/' + (model?.model || 'convaiinnovations/laya-typed-decisions')} target="_blank" rel="noreferrer">{model?.model || 'convaiinnovations/laya-typed-decisions'} ↗</a><span>{ready ? model.device_name + ' · FP32 · ' + model.context_tokens + ' tokens' : 'Loading and warming models…'}</span></div>
  </div>

  <div class="workspace">
    <section class="card input-card" aria-labelledby="input-title">
      <div class="card-heading"><h2 id="input-title"><span class="step">01</span> Set up the questions</h2><span class="subtle">Native Laya format</span></div>
      <form onsubmit={run}>
        <fieldset disabled={busy}>
          <div class="field">
            <label for="model-select">Model</label>
            <select id="model-select" bind:value={modelKey} onchange={clearResult}><option value="typed-decisions">Laya Typed-Decisions · specialist</option><option value="english">Laya General English · original</option></select>
            <p class="field-hint">{model?.description || 'Choose a checkpoint to run locally.'}</p>
          </div>
          <div class="examples">
            <div class="label-row"><label for="example-search">Try an example</label><span class="subtle" aria-live="polite">{filteredExamples.length} of {Object.keys(examples).length}</span></div>
            <input id="example-search" type="search" placeholder="Search examples, e.g. support, documents, security" autocomplete="off" bind:value={query} onkeydown={(event) => { if (event.key === 'Enter') event.preventDefault(); }} />
            <div class="example-buttons" role="group" aria-label="Decision examples">
              {#each filteredExamples as [key, example]}
                <button type="button" class="example" class:active={selectedExample === key} aria-pressed={selectedExample === key} data-example={key} title={example.category} onclick={() => loadExample(key)}>{example.title}</button>
              {/each}
            </div>
            {#if examplesLoaded && !filteredExamples.length}<p class="field-hint">No examples match. Try another keyword.</p>{/if}
            {#if examples[selectedExample]?.hint}<p class="field-hint">{examples[selectedExample].hint}</p>{/if}
          </div>
          <div class="field">
            <JsonEditor id="state-json" label="State" kind="state" bind:value={stateJSON} disabled={busy} onchange={edited} />
            <p class="field-hint">The same state is supplied to every question. Keep all details needed for the decision here.</p>
          </div>
          <div class="field">
            <JsonEditor id="questions-json" label="Questions" kind="questions" bind:value={questionsJSON} disabled={busy} onchange={edited} />
            <p class="field-hint"><code>noul</code> = yes/no decision · <code>choice</code> = select an option · <code>score</code> = a position on your ordered rubric. Ask up to 8 questions together.</p>
            <p class="field-hint">{model?.context_tokens || 1024} tokens per question, including its state, instructions, and options.</p>
          </div>
        </fieldset>
        {#if error || model?.error}<div class="error" role="alert">{error || model.error}</div>{/if}
        <div class="form-footer">
          <button id="run" class="primary" type="submit" disabled={!canRun}><span>{busy ? 'Answering questions…' : !examplesLoaded ? 'Loading examples…' : !valid ? 'Fix JSON to continue' : !ready ? 'Waiting for model…' : 'Run all questions'}</span><span id="run-icon" aria-hidden="true">{busy ? '· · ·' : '↗'}</span></button>
          <span class="keyboard-hint"><kbd>Ctrl</kbd> + <kbd>Enter</kbd> to run</span>
        </div>
      </form>
    </section>

    <section bind:this={resultCard} id="result-card" class="card result-card" aria-labelledby="result-title" aria-busy={busy}>
      <div class="card-heading"><h2 id="result-title"><span class="step">02</span> See the answers</h2><span class="result-status" class:complete={!!result} role="status">{busy ? 'Running locally' : result ? Object.keys(result.answers).length + ' answers' : 'Awaiting input'}</span></div>
      {#if result}
        <div class="result-content">
          <p class="result-model">{result.model}</p>
          <div id="answers">
            {#each Object.entries(lastRequest.questions) as [name, question]}
              <DecisionCard {name} {question} answer={result.answers[name]} />
            {/each}
          </div>
          <div class="metrics"><div><span>Inference time</span><strong>{result.elapsed_ms >= 1000 ? (result.elapsed_ms / 1000).toFixed(2) + ' s' : result.elapsed_ms.toFixed(1) + ' ms'}</strong></div><div><span>Total input tokens</span><strong>{result.usage.input_tokens.toLocaleString()}</strong></div><div><span>Device / precision</span><strong>{result.device.toUpperCase()} / FP32</strong></div></div>
          <p class="result-note">Answers are model estimates. Scores use zero-based rubric levels. Input tokens include the state repeated for each question.</p>
          <details class="api-details" bind:open={showAPI}>
            <summary>View API request & response <span aria-hidden="true">↗</span></summary>
            {#if showAPI}
              <JsonEditor id="request-json" label="POST /api/predict" value={JSON.stringify(lastRequest, null, 2)} readOnly />
              <JsonEditor id="response-json" label="Response" value={JSON.stringify(result, null, 2)} readOnly />
            {/if}
          </details>
        </div>
      {:else}
        <div class="empty-result">
          <svg class="decision-graphic" viewBox="0 0 200 140" fill="none" aria-hidden="true"><path d="M100 42v28M38 90V70h124v20M100 70v20" stroke="#c6d5cf" stroke-width="2" /><rect x="76" y="4" width="48" height="42" rx="12" fill="#e7f0eb" stroke="#bdcfc5" /><path d="m88 25 8 8 16-17" stroke="#397c60" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" /><rect x="16" y="90" width="44" height="34" rx="9" fill="#f3f5f4" stroke="#d6dfda" /><rect x="78" y="90" width="44" height="34" rx="9" fill="#e7f0eb" stroke="#a3c3b1" /><rect x="140" y="90" width="44" height="34" rx="9" fill="#f3f5f4" stroke="#d6dfda" /><circle cx="38" cy="107" r="3" fill="#bcc9c1" /><circle cx="100" cy="107" r="4" fill="#397c60" /><circle cx="162" cy="107" r="3" fill="#bcc9c1" /></svg>
          <h3>{busy ? 'Laya is answering the questions…' : 'From state to decisions.'}</h3>
          <p>{busy ? 'The state and questions are evaluated together.' : 'Choose an example and run its questions. Each decision appears here, with the winning answer highlighted.'}</p>
          <div class="process-labels"><span>State + questions</span><span aria-hidden="true">→</span><span>Laya</span><span aria-hidden="true">→</span><span>Answers</span></div>
        </div>
      {/if}
      <div class="result-footer"><span class="local-dot" aria-hidden="true"></span> Your prompt stays on this machine.</div>
    </section>
  </div>
  <footer class="page-footer"><span>Both checkpoints load and warm locally. One request answers all its questions.</span><a href="/docs">Local API docs ↗</a></footer>
</main>
