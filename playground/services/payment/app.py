from http.server import HTTPServer, BaseHTTPRequestHandler
import json
import os


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/health":
            self._json(200, {"status": "healthy"})
        else:
            self._json(200, {"service": "payment", "version": os.getenv("BUILD_VERSION", "unknown")})

    def _json(self, code, body):
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(json.dumps(body).encode())


if __name__ == "__main__":
    port = int(os.getenv("PORT", "8081"))
    server = HTTPServer(("", port), Handler)
    print(f"payment-service listening on :{port}")
    server.serve_forever()
