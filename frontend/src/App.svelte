<script>
  import { onMount, tick } from 'svelte';
  import JsonEditor from './JsonEditor.svelte';
  import DecisionCard from './DecisionCard.svelte';
  import { inspectJSON } from './lib/json.js';

  let examples = $state({ laya: {}, jev: {} });
  let workspaces = $state({
    laya: { selectedExample: 'refund', query: '', stateJSON: '{}', questionsJSON: '{}', result: null, lastRequest: null, showAPI: false },
    jev: { selectedExample: 'incident_triage', query: '', stateJSON: '{}', questionsJSON: '{}', result: null, lastRequest: null, showAPI: false },
  });
  let activeTab = $state('laya');
  let models = $state([]);
  let modelKey = $state('typed-decisions');
  let apiOnline = $state(false);
  let jevConfig = $state({ loaded: false, configured: false, model: 'jev-latest' });
  let examplesLoaded = $state({ laya: false, jev: false });
  let busy = $state(false);
  let error = $state('');
  let resultCard;
  const current = $derived(workspaces[activeTab]);
  const currentExamples = $derived(examples[activeTab]);
  const model = $derived(models.find((item) => item.key === modelKey));
  const ready = $derived(activeTab === 'laya' ? apiOnline && model?.status === 'ready' : jevConfig.loaded && jevConfig.configured);
  const stateCheck = $derived(inspectJSON(current.stateJSON, 'state'));
  const questionsCheck = $derived(inspectJSON(current.questionsJSON, 'questions'));
  const valid = $derived(stateCheck.valid && questionsCheck.valid);
  const canRun = $derived(ready && examplesLoaded[activeTab] && valid && !busy);
  const filteredExamples = $derived(Object.entries(currentExamples).filter(([, example]) => (example.title + ' ' + example.category).toLowerCase().includes(current.query.trim().toLowerCase())));
  const connectionStatus = $derived(activeTab === 'laya' ? (!apiOnline ? 'offline' : model?.status || 'loading') : (!jevConfig.loaded ? 'loading' : jevConfig.configured ? 'configured' : 'missing'));
  const connectionLabel = $derived(activeTab === 'laya'
    ? (!apiOnline ? 'Connecting to local API' : ready ? 'Local model ready' : model?.status === 'error' ? 'Model unavailable' : 'Loading and warming…')
    : (!jevConfig.loaded ? 'Checking Jev configuration…' : jevConfig.configured ? 'Jev API key configured' : 'Add Jev key in .env'));

  function clearResult() { current.result = null; current.showAPI = false; error = ''; }
  function edited() { current.selectedExample = ''; clearResult(); }
  function loadExample(key) {
    if (busy) return;
    const example = currentExamples[key];
    if (!example) return;
    current.stateJSON = JSON.stringify(example.state, null, 2);
    current.questionsJSON = JSON.stringify(example.questions, null, 2);
    current.selectedExample = key;
    clearResult();
  }
  function selectTab(tab) {
    if (busy || tab === activeTab) return;
    activeTab = tab;
    error = '';
  }

  onMount(() => {
    let stopped = false;
    let timer;
    const controller = new AbortController();
    async function health() {
      try {
        const signal = AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]);
        const [layaResponse, jevResponse] = await Promise.all([fetch('/api/health', { signal, cache: 'no-store' }), fetch('/api/jev/health', { signal, cache: 'no-store' })]);
        if (!layaResponse.ok || !jevResponse.ok) throw new Error('API unavailable');
        const [layaData, jevData] = await Promise.all([layaResponse.json(), jevResponse.json()]);
        if (!stopped) {
          models = layaData.models;
          apiOnline = true;
          jevConfig = { loaded: true, configured: jevData.configured, model: jevData.model };
        }
      } catch {
        if (!stopped) { apiOnline = false; jevConfig = { ...jevConfig, loaded: true, configured: false }; }
      } finally { if (!stopped) timer = setTimeout(health, apiOnline && models.every((item) => item.status !== 'loading') ? 15000 : 2000); }
    }
    async function loadExamples(tab, path) {
      try {
        const response = await fetch(path, { signal: controller.signal });
        if (!response.ok) throw new Error('Could not load the examples. Refresh the page.');
        const data = await response.json();
        if (!stopped) {
          examples[tab] = data;
          examplesLoaded[tab] = true;
          const initial = tab === 'laya' ? 'refund' : 'incident_triage';
          const example = data[initial];
          workspaces[tab].stateJSON = JSON.stringify(example.state, null, 2);
          workspaces[tab].questionsJSON = JSON.stringify(example.questions, null, 2);
          workspaces[tab].selectedExample = initial;
        }
      } catch (err) { if (!stopped) error = err.message; }
    }
    health();
    loadExamples('laya', '/examples.json');
    loadExamples('jev', '/jev-examples.json');
    return () => { stopped = true; clearTimeout(timer); controller.abort(); };
  });

  async function run(event) {
    event?.preventDefault();
    if (!canRun) return;
    const tab = activeTab;
    const request = { model: tab === 'jev' ? jevConfig.model : modelKey, state: stateCheck.value, questions: questionsCheck.value };
    workspaces[tab].lastRequest = request;
    clearResult();
    busy = true;
    try {
      const response = await fetch(tab === 'jev' ? '/api/jev' : '/api/predict', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(request), signal: AbortSignal.timeout(125000),
      });
      const data = await response.json();
      if (!response.ok) throw new Error(typeof data.detail === 'string' ? data.detail : 'The request could not be completed.');
      workspaces[tab].result = data;
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
  <a class="brand" href="/" aria-label="Decision playground home"><img src="/icon.svg" width="34" height="34" alt="" /><span>decision<span class="brand-divider">/</span><span class="brand-caption">playground</span></span></a>
  <nav class="provider-tabs" role="tablist" aria-label="Decision model">
    <button id="tab-laya" type="button" role="tab" aria-selected={activeTab === 'laya'} aria-controls="provider-panel" class:active={activeTab === 'laya'} disabled={busy} onclick={() => selectTab('laya')}>Laya</button>
    <button id="tab-jev" type="button" role="tab" aria-selected={activeTab === 'jev'} aria-controls="provider-panel" class:active={activeTab === 'jev'} disabled={busy} onclick={() => selectTab('jev')}>Jev</button>
  </nav>
  <div class="connection" data-status={connectionStatus} role="status"><span class="status-dot"></span><span>{connectionLabel}</span></div>
</header>

<main id="provider-panel" role="tabpanel" aria-labelledby={'tab-' + activeTab}>
  <div class="intro">
    <div><p class="eyebrow">ONE STATE. SEVERAL DECISIONS.</p><h1>{activeTab === 'jev' ? 'Make focused decisions.' : 'Make the next call.'}</h1><p class="lede">{#if activeTab === 'jev'}Give Jev one state and several focused questions.<br />Inspect typed answers, probabilities, and confidence.{:else}Give Laya structured context and typed questions.<br />Explore yes/no decisions, choices, and scores in one pass.{/if}</p></div>
    <div class="model-note">
      {#if activeTab === 'jev'}
        <span class="model-note-label">HOSTED BY TYPESAFE</span><a href="https://docs.typesafe.ai/api" target="_blank" rel="noreferrer">{jevConfig.model} ↗</a><span>{jevConfig.configured ? 'API key configured on server' : 'Set TYPESAFE_API_KEY in .env, then restart'}</span>
      {:else}
        <span class="model-note-label">RUNNING LOCALLY</span><a href={'https://huggingface.co/' + (model?.model || 'convaiinnovations/laya-typed-decisions')} target="_blank" rel="noreferrer">{model?.model || 'convaiinnovations/laya-typed-decisions'} ↗</a><span>{ready ? model.device_name + ' · FP32 · ' + model.context_tokens + ' tokens' : 'Loading and warming models…'}</span>
      {/if}
    </div>
  </div>

  <div class="workspace">
    <section class="card input-card" aria-labelledby="input-title">
      <div class="card-heading"><h2 id="input-title"><span class="step">01</span> Set up the questions</h2><span class="subtle">{activeTab === 'jev' ? 'Jev typed decisions' : 'Native Laya format'}</span></div>
      <form onsubmit={run}>
        <fieldset disabled={busy}>
          {#if activeTab === 'laya'}
            <div class="field">
              <label for="model-select">Model</label>
              <select id="model-select" bind:value={modelKey} onchange={clearResult}><option value="typed-decisions">Laya Typed-Decisions · specialist</option><option value="english">Laya General English · original</option></select>
              <p class="field-hint">{model?.description || 'Choose a checkpoint to run locally.'}</p>
            </div>
          {:else}
            <div class="field">
              <label for="jev-model">Jev model</label>
              <input id="jev-model" value={jevConfig.model} readonly />
              <p class="field-hint">The API key is read by the Go server from the root .env file. It is never sent to the browser.</p>
            </div>
          {/if}
          <div class="examples">
            <div class="label-row"><label for="example-search">Try an example</label><span class="subtle" aria-live="polite">{filteredExamples.length} of {Object.keys(currentExamples).length}</span></div>
            <input id="example-search" type="search" placeholder={activeTab === 'jev' ? 'Search Jev examples' : 'Search examples, e.g. support, documents, security'} autocomplete="off" bind:value={workspaces[activeTab].query} onkeydown={(event) => { if (event.key === 'Enter') event.preventDefault(); }} />
            <div class="example-buttons" role="group" aria-label={activeTab + ' decision examples'}>
              {#each filteredExamples as [key, example]}
                <button type="button" class="example" class:active={current.selectedExample === key} aria-pressed={current.selectedExample === key} data-example={key} title={example.category} onclick={() => loadExample(key)}>{example.title}</button>
              {/each}
            </div>
            {#if examplesLoaded[activeTab] && !filteredExamples.length}<p class="field-hint">No examples match. Try another keyword.</p>{/if}
            {#if currentExamples[current.selectedExample]?.hint}<p class="field-hint">{currentExamples[current.selectedExample].hint}</p>{/if}
          </div>
          <div class="field">
            <JsonEditor id="state-json" label="State" kind="state" bind:value={workspaces[activeTab].stateJSON} disabled={busy} onchange={edited} />
            <p class="field-hint">The same state is supplied to every question. Keep all details needed for the decision here.</p>
          </div>
          <div class="field">
            <JsonEditor id="questions-json" label="Questions" kind="questions" bind:value={workspaces[activeTab].questionsJSON} disabled={busy} onchange={edited} />
            <p class="field-hint"><code>noul</code> = yes/no decision · <code>choice</code> = select an option · <code>score</code> = a position on your ordered rubric. Ask up to 8 questions together.</p>
            {#if activeTab === 'jev'}<p class="field-hint">Each question is evaluated independently against the shared state in one API call.</p>{:else}<p class="field-hint">{model?.context_tokens || 1024} tokens per question, including its state, instructions, and options.</p>{/if}
          </div>
        </fieldset>
        {#if error || activeTab === 'laya' && model?.error}<div class="error" role="alert">{error || model.error}</div>{/if}
        <div class="form-footer">
          <button id="run" class="primary" type="submit" disabled={!canRun}><span>{busy ? (activeTab === 'jev' ? 'Asking Jev…' : 'Answering questions…') : !examplesLoaded[activeTab] ? 'Loading examples…' : !valid ? 'Fix JSON to continue' : !ready ? (activeTab === 'jev' && jevConfig.loaded ? 'Configure Jev in .env' : 'Waiting for model…') : activeTab === 'jev' ? 'Run with Jev' : 'Run all questions'}</span><span id="run-icon" aria-hidden="true">{busy ? '· · ·' : '↗'}</span></button>
          <span class="keyboard-hint"><kbd>Ctrl</kbd> + <kbd>Enter</kbd> to run</span>
        </div>
      </form>
    </section>

    <section bind:this={resultCard} id="result-card" class="card result-card" aria-labelledby="result-title" aria-busy={busy}>
      <div class="card-heading"><h2 id="result-title"><span class="step">02</span> See the answers</h2><span class="result-status" class:complete={!!current.result} role="status">{busy ? (activeTab === 'jev' ? 'Calling Jev' : 'Running locally') : current.result ? Object.keys(current.result.answers).length + ' answers' : 'Awaiting input'}</span></div>
      {#if current.result}
        <div class="result-content">
          <p class="result-model">{current.result.model}</p>
          <div id="answers">
            {#each Object.entries(current.lastRequest.questions) as [name, question]}
              <DecisionCard {name} {question} answer={current.result.answers[name]} />
            {/each}
          </div>
          <div class="metrics"><div><span>{activeTab === 'jev' ? 'API response time' : 'Inference time'}</span><strong>{current.result.elapsed_ms >= 1000 ? (current.result.elapsed_ms / 1000).toFixed(2) + ' s' : current.result.elapsed_ms.toFixed(1) + ' ms'}</strong></div><div><span>Total input tokens</span><strong>{current.result.usage.input_tokens.toLocaleString()}</strong></div><div><span>{activeTab === 'jev' ? 'Model confidence' : 'Device / precision'}</span><strong>{activeTab === 'jev' ? 'Per answer' : current.result.device.toUpperCase() + ' / FP32'}</strong></div></div>
          <p class="result-note">Answers are model estimates. Choice and Score confidence are model signals, not guarantees. Scores use zero-based rubric levels.</p>
          <details class="api-details" bind:open={workspaces[activeTab].showAPI}>
            <summary>View API request & response <span aria-hidden="true">↗</span></summary>
            {#if current.showAPI}
              <JsonEditor id="request-json" label={activeTab === 'jev' ? 'POST /api/jev' : 'POST /api/predict'} value={JSON.stringify(current.lastRequest, null, 2)} readOnly />
              <JsonEditor id="response-json" label="Response" value={JSON.stringify(current.result, null, 2)} readOnly />
            {/if}
          </details>
        </div>
      {:else}
        <div class="empty-result">
          <svg class="decision-graphic" viewBox="0 0 200 140" fill="none" aria-hidden="true"><path d="M100 42v28M38 90V70h124v20M100 70v20" stroke="#c6d5cf" stroke-width="2" /><rect x="76" y="4" width="48" height="42" rx="12" fill="#e7f0eb" stroke="#bdcfc5" /><path d="m88 25 8 8 16-17" stroke="#397c60" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" /><rect x="16" y="90" width="44" height="34" rx="9" fill="#f3f5f4" stroke="#d6dfda" /><rect x="78" y="90" width="44" height="34" rx="9" fill="#e7f0eb" stroke="#a3c3b1" /><rect x="140" y="90" width="44" height="34" rx="9" fill="#f3f5f4" stroke="#d6dfda" /><circle cx="38" cy="107" r="3" fill="#bcc9c1" /><circle cx="100" cy="107" r="4" fill="#397c60" /><circle cx="162" cy="107" r="3" fill="#bcc9c1" /></svg>
          <h3>{busy ? (activeTab === 'jev' ? 'Jev is answering the questions…' : 'Laya is answering the questions…') : 'From state to decisions.'}</h3>
          <p>{busy ? 'The state and questions are evaluated together.' : 'Choose an example and run its questions. Each decision appears here, with the winning answer highlighted.'}</p>
          <div class="process-labels"><span>State + questions</span><span aria-hidden="true">→</span><span>{activeTab === 'jev' ? 'Jev' : 'Laya'}</span><span aria-hidden="true">→</span><span>Answers</span></div>
        </div>
      {/if}
      <div class="result-footer"><span class="local-dot" aria-hidden="true"></span>{activeTab === 'jev' ? 'State and questions are sent to TypeSafe’s Jev API.' : 'Your prompt stays on this machine.'}</div>
    </section>
  </div>
  <footer class="page-footer"><span>{activeTab === 'jev' ? 'Jev returns typed decisions; your app decides what to do with them.' : 'Both checkpoints load and warm locally. One request answers all its questions.'}</span><a href="/docs">Local API docs ↗</a></footer>
</main>
