<script>
  let { name, answer, question } = $props();
  const percent = (value) => (value * 100).toFixed(1) + '%';
  const yes = $derived(question.labels?.true || 'Yes');
  const no = $derived(question.labels?.false || 'No');
  const tied = $derived(answer.type === 'noul' && answer.noul === 0.5);
  const winner = $derived(answer.type === 'noul' ? (tied ? null : answer.noul > 0.5 ? 'true' : 'false') : answer.choice);
  const rows = $derived.by(() => {
    if (answer.type === 'noul') return [
      { key: 'true', label: yes, probability: answer.noul },
      { key: 'false', label: no, probability: 1 - answer.noul },
    ];
    if (answer.type === 'score') return Object.entries(answer.legend || question.criteria).map(([key, label]) => ({ key, label: key + ' — ' + label, probability: answer.probabilities[key] }));
    return Object.entries(answer.probabilities).map(([key, probability]) => ({ key, probability, label: key + (!Array.isArray(question.criteria) && question.criteria[key] ? ' — ' + question.criteria[key] : '') })).sort((a, b) => b.probability - a.probability);
  });
  const topScore = $derived(answer.type === 'score' ? rows.reduce((best, row) => row.probability > best.probability ? row : best).key : null);
</script>

<article class="answer-card" data-question={name}>
  <div class="answer-heading"><h3 class="answer-name">{name}</h3><div class="answer-tags"><span class="type-tag">{answer.type}</span>{#if typeof answer.confidence === 'number'}<span class="confidence-tag">{percent(answer.confidence)} confidence</span>{/if}</div></div>
  <p class="answer-instructions">{question.instructions}</p>
  {#if answer.type === 'noul'}
    <div class="winner-heading">
      <strong class="answer-value">{tied ? 'No clear winner' : winner === 'true' ? yes : no}</strong>
      <span class="winner-badge" class:tie={tied}>{tied ? 'Tie' : '✓ Winner'}</span>
    </div>
    <p class="answer-explanation">{tied ? 'Both answers have equal support.' : 'The model selected this answer.'}</p>
    {#if question.criteria?.true || question.criteria?.false}<p class="noul-criteria">Yes: {question.criteria.true || 'yes'} · No: {question.criteria.false || 'no'}</p>{/if}
    <div class="boolean-options" aria-label="Yes or no result">
      {#each rows as row}
        <div class="boolean-option" class:selected={winner === row.key}>
          <span>{row.label}</span>
          <strong>{percent(row.probability)}</strong>
          <small>{winner === row.key ? 'Winner' : tied ? 'Equal support' : 'Alternative'}</small>
        </div>
      {/each}
    </div>
  {:else}
    <div class="answer-value">{answer.type === 'choice' ? answer.choice : answer.score.toFixed(2) + ' / ' + (rows.length - 1)}</div>
    <p class="answer-explanation">{answer.type === 'choice' ? percent(answer.probabilities[answer.choice]) + ' support for the selected choice' : 'Expected rubric level · levels start at 0'}</p>
    <div class="answer-distribution">
      {#each rows as row}
        <div class="probability-row" class:selected={row.key === (answer.type === 'score' ? topScore : winner)}>
          <div class="probability-caption"><span class="probability-label">{row.label}</span><span class="probability-value">{percent(row.probability)}</span></div>
          <div class="track" aria-hidden="true"><div class="fill" style:width={Math.max(0, Math.min(100, row.probability * 100)) + '%'}></div></div>
        </div>
      {/each}
    </div>
  {/if}
</article>
