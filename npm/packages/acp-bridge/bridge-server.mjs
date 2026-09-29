#!/usr/bin/env node
import { serveAcpService } from '@1agents/acp-service';

const service = await serveAcpService({
  port: Number(process.env.ACPX_PORT || 38082),
  host: '127.0.0.1',
  report: false,
});
let closing = false;
async function close() {
  if (closing) return;
  closing = true;
  try { await service.close(); }
  finally { process.stdin.pause(); }
}
process.stdin.resume();
process.stdin.once('end', () => void close());
process.once('SIGINT', () => void close());
process.once('SIGTERM', () => void close());
