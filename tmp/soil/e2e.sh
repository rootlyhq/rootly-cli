#!/bin/bash
# end-to-end: built binary against a fake API; stdout and stderr captured separately
cd /workspace/lane
go build -o /tmp/rootly-e2e ./cmd/rootly || exit 1
cat > /tmp/fakeapi.py <<'PY'
import http.server, sys
class H(http.server.BaseHTTPRequestHandler):
    def _send(self, code, body):
        self.send_response(code); self.send_header("Content-Type","application/vnd.api+json"); self.end_headers(); self.wfile.write(body.encode())
    def handle_any(self):
        n=int(self.headers.get("Content-Length") or 0); self.rfile.read(n)
        sys.stderr.write("REQ %s %s\n" % (self.command, self.path)); sys.stderr.flush()
        if "/status-page-events" in self.path:
            if MODE == "fail": return self._send(500, "{}")
            return self._send(200, '{"data":[{"id":"ev-1","attributes":{"event":"Investigating","status":"investigating","status_page_id":"page-1"}},{"id":"ev-2","attributes":{"event":"Done","status":"completed","status_page_id":"page-1"}}],"meta":{"current_page":1,"total_pages":1}}')
        self._send(200, '{"data":{"id":"inc-1","attributes":{"sequential_id":42,"title":"DB outage","status":"%s","kind":"normal","created_at":"2025-06-15T10:00:00Z"}}}' % STATUS)
    do_GET=do_PATCH=do_POST=do_PUT=handle_any
    def log_message(self,*a): pass
MODE=sys.argv[2]; STATUS=sys.argv[3]
http.server.HTTPServer(("127.0.0.1",int(sys.argv[1])),H).serve_forever()
PY
run() { # name port mode status
  python3 -I /tmp/fakeapi.py $2 $3 $4 2>/tmp/req-$1.log & pid=$!
  sleep 1
  echo "##### $1 (events lookup: $3, API returns status: $4)"
  HOME=/tmp/e2ehome ROOTLY_API_KEY=k ROOTLY_API_HOST=http://127.0.0.1:$2 /tmp/rootly-e2e incidents update inc-1 --status=$4 --format json >/tmp/out-$1.txt 2>/tmp/err-$1.txt
  echo "exit=$?"
  echo "--- stdout:"; cat /tmp/out-$1.txt
  echo "--- stderr:"; cat /tmp/err-$1.txt
  echo "--- requests made:"; cat /tmp/req-$1.log
  kill $pid; wait $pid 2>/dev/null
}
mkdir -p /tmp/e2ehome
run resolved 18081 ok resolved
run lookup-fails 18082 fail resolved
run not-resolved 18083 ok mitigated
