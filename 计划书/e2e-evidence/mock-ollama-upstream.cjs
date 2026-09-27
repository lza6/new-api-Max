const http = require('http');
const srv = http.createServer((req, res) => {
  let body = '';
  req.on('data', (c) => (body += c));
  req.on('end', () => {
    if (req.url === '/api/tags') {
      res.setHeader('Content-Type', 'application/json');
      res.end(JSON.stringify({ models: [{ name: 'llama3' }] }));
      return;
    }
    if (req.url === '/api/chat' || req.url === '/api/generate') {
      res.setHeader('Content-Type', 'application/json');
      const b = { model: 'llama3', message: { role: 'assistant', content: 'hello from mock ollama upstream' }, done: true };
      res.end(JSON.stringify(b));
      return;
    }
    res.statusCode = 404;
    res.end('{}');
  });
});
srv.listen(18999, '127.0.0.1', () => console.log('mock-ollama up on 18999'));
process.on('SIGTERM', () => srv.close(() => process.exit(0)));
