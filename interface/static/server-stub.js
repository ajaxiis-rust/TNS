/* server-stub.js — TNS65 interface copy (16.10.2026).
   All calls to the old engine server were removed from this copy.
   Former fetch() sites now call tnsStubFetch() below.
   To wire a real backend: replace these stub bodies, nothing else. */
function tnsStubFetch() {
  return Promise.resolve({
    ok: true,
    status: 200,
    statusText: 'Stub',
    headers: { get: function () { return null; } },
    json: function () { return Promise.resolve({}); },
    text: function () { return Promise.resolve(''); }
  });
}
function tnsStubEventSource() {
  return { addEventListener: function () {}, close: function () {} };
}
function tnsStubWebSocket() {
  return { addEventListener: function () {}, send: function () {}, close: function () {} };
}
