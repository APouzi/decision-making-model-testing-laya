import { parseTree, findNodeAtLocation, printParseErrorCode } from 'jsonc-parser';

const object = (value) => value !== null && typeof value === 'object' && !Array.isArray(value);
const text = (value, max) => typeof value === 'string' && value.trim().length > 0 && [...value].length <= max;

// The same analysis feeds the editor gutter, readable messages, and Run button.
export function inspectJSON(source, kind = 'json') {
  const errors = [];
  const tree = parseTree(source, errors, { disallowComments: true, allowTrailingComma: false });
  const issues = errors.map((error) => ({
    from: error.offset, to: error.offset + error.length,
    severity: 'error', message: printParseErrorCode(error.error).replace(/([a-z])([A-Z])/g, '$1 $2'),
  }));
  function report(message, path = []) {
    const node = tree && findNodeAtLocation(tree, path);
    issues.push({ from: node?.offset ?? 0, to: (node?.offset ?? 0) + (node?.length ?? 0), severity: 'error', message });
  }
  function duplicates(node) {
    if (node?.type === 'object') {
      const keys = new Set();
      for (const property of node.children || []) {
        const key = property.children[0];
        if (keys.has(key.value)) issues.push({ from: key.offset, to: key.offset + key.length, severity: 'error', message: 'Duplicate key: ' + key.value });
        keys.add(key.value);
      }
    }
    for (const child of node?.children || []) duplicates(child);
  }
  duplicates(tree);
  let value;
  if (!issues.length) {
    try { value = JSON.parse(source); }
    catch { report('Enter valid JSON.'); }
  }
  if (!issues.length && kind === 'state') {
    if (!(object(value) || Array.isArray(value) || typeof value === 'string')) report('State must be an object, array, or string.');
    else if (typeof value === 'string' ? !value.trim() : !Object.keys(value).length) report('Provide a non-empty state.');
    if ([...JSON.stringify(value)].length > 20000) report('Keep state under 20,000 characters.');
  }
  if (!issues.length && kind === 'questions') {
    if (!object(value)) report('Questions must be an object keyed by question name.');
    else {
      const entries = Object.entries(value);
      if (entries.length < 1 || entries.length > 8) report('Provide 1 to 8 named questions.');
      for (const [name, q] of entries) {
        const at = (message, key) => report(name + ': ' + message, key ? [name, key] : [name]);
        if (!text(name, 80)) at('Use a question name of 1–80 characters.');
        if (!object(q)) { at('A question must be an object.'); continue; }
        for (const key of Object.keys(q)) if (!['type', 'instructions', 'criteria', 'labels'].includes(key)) at('Unknown field ' + key + '.', key);
        if (!['noul', 'choice', 'score'].includes(q.type)) at('Type must be noul, choice, or score.', 'type');
        if (!text(q.instructions, 600)) at('Instructions need 1–600 characters.', 'instructions');
        if (q.type === 'choice' || q.type === 'score') {
          const valid = Array.isArray(q.criteria) || (q.type === 'choice' && object(q.criteria));
          if (!valid) at(q.type === 'score' ? 'Score criteria must be an ordered list.' : 'Choice criteria must be an object or list.', 'criteria');
          else {
            const options = Object.values(q.criteria);
            if (options.length < 2 || options.length > 8) at('Use 2–8 criteria.', 'criteria');
            if (options.some((option) => !text(option, 300))) at('Each criterion needs 1–300 characters.', 'criteria');
            if (object(q.criteria) && Object.keys(q.criteria).some((key) => !text(key, 80))) at('Option names need 1–80 characters.', 'criteria');
            if (Array.isArray(q.criteria) && new Set(options.map((v) => typeof v === 'string' ? v.trim() : v)).size !== options.length) at('Give every criterion distinct wording.', 'criteria');
          }
          if (q.labels != null) at('Labels are only supported for noul.', 'labels');
        }
        if (q.type === 'noul') {
          if (q.criteria != null && (!object(q.criteria) || Object.entries(q.criteria).some(([key, v]) => !['false', 'true'].includes(key) || !text(v, 300)))) at('Noul criteria use false/true keys and text descriptions.', 'criteria');
          if (q.labels != null && (!object(q.labels) || Object.keys(q.labels).length !== 2 || !text(q.labels.false, 300) || !text(q.labels.true, 300) || q.labels.false.trim() === q.labels.true.trim())) at('Labels must map false and true to different text.', 'labels');
        }
      }
    }
  }
  return { value, issues, valid: issues.length === 0 };
}

export function position(source, offset) {
  const lines = source.slice(0, offset).split('\n');
  return 'Line ' + lines.length + ', column ' + (lines.at(-1).length + 1);
}
