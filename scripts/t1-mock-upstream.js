// T1 bench mock upstream: fixed 30ms latency + OpenAI-compatible response
import http from 'node:http';
const server = http.createServer((req, res) => {
  let body = '';
  req.on('data', (c) => { body += c; });
  req.on('end', () => {
    setTimeout(() => {
      res.writeHead(200, { 'Content-Type': 'application/json', 'X-Oneapi-Request-Id': req.headers['x-oneapi-request-id'] || 'mock-no-id' });
      res.end(JSON.stringify({ id: 'chatcmpl-mock', object: 'chat.completion', created: 0, model: 'bench-model', choices: [{ index: 0, message: { role: 'assistant', content: 'ok' }, finish_reason: 'stop' }], usage: { prompt_tokens: 2, completion_tokens: 2, total_tokens: 4 } }));
    }, 30);
  });
});
server.listen(18080, '127.0.0.1', () => console.log('mock upstream on 18080'));
