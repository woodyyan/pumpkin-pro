export const config = {
  api: {
    bodyParser: false,
  },
};

const DEFAULT_BACKEND_API_URL = 'http://localhost:8080';
export const FORWARDED_REQUEST_HEADERS = ['content-type', 'accept', 'authorization', 'cookie', 'user-agent', 'x-forwarded-for', 'x-real-ip'];

export default async function handler(req, res) {
  const backendApiUrl = (process.env.BACKEND_API_URL || DEFAULT_BACKEND_API_URL).replace(/\/$/, '');
  const pathSegments = Array.isArray(req.query.path) ? req.query.path : [req.query.path].filter(Boolean);
  const requestUrl = new URL(req.url, 'http://localhost');
  const targetUrl = `${backendApiUrl}/api/${pathSegments.join('/')}${requestUrl.search}`;

  try {
    const body = shouldReadBody(req.method) ? await readRawBody(req) : undefined;
    const upstreamResponse = await fetch(targetUrl, {
      method: req.method,
      headers: buildForwardHeaders(req),
      body: shouldSendBody(body) ? body : undefined,
    });

    const responseText = await upstreamResponse.text();
    const upstreamContentType = upstreamResponse.headers.get('content-type') || 'application/json; charset=utf-8';

    res.status(upstreamResponse.status);
    res.setHeader('Content-Type', upstreamContentType);
    copySetCookieHeaders(upstreamResponse, res);

    if (upstreamResponse.ok || upstreamContentType.includes('application/json')) {
      res.send(responseText);
      return;
    }

    res.send(JSON.stringify({ detail: responseText || `请求后端失败：${upstreamResponse.status}` }));
  } catch (error) {
    console.error(`Failed to proxy ${targetUrl}`, error);
    res.status(503).json({
      detail: `无法连接后端服务，请确认 BACKEND_API_URL=${backendApiUrl} 且后端服务已启动。`,
    });
  }
}

export function buildForwardHeaders(req) {
  const headers = {};
  FORWARDED_REQUEST_HEADERS.forEach((headerName) => {
    const value = req.headers[headerName];
    if (value) {
      headers[headerName] = value;
    }
  });
  return headers;
}

function copySetCookieHeaders(upstreamResponse, res) {
  const header = upstreamResponse.headers.get('set-cookie');
  if (!header) return;
  res.setHeader('Set-Cookie', splitSetCookieHeader(header));
}

export function splitSetCookieHeader(value) {
  if (!value) return [];
  return value.split(/,(?=\s*[^;=]+=[^;]+)/g).map((item) => item.trim()).filter(Boolean);
}

function shouldReadBody(method) {
  return !['GET', 'HEAD'].includes((method || '').toUpperCase());
}

function shouldSendBody(body) {
  return body && body.length > 0;
}

async function readRawBody(req) {
  const chunks = [];
  for await (const chunk of req) {
    chunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk));
  }
  return Buffer.concat(chunks);
}
