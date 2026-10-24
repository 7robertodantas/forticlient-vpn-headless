#!/usr/bin/env python3
"""Forward 127.0.0.1:PORT (and [::1]:PORT) on this host into the gateway.

Each accepted connection runs CMD (e.g. `docker exec -i fct-headless socat
STDIO TCP:127.0.0.1:PORT`) and pipes the socket through its stdin/stdout.
The transport command is what makes this work for Docker, Lima or SSH alike.

Usage: saml-forward.py PORT -- CMD [ARG...]
"""
import asyncio
import socket
import sys


async def pump(reader, writer):
    try:
        while data := await reader.read(65536):
            writer.write(data)
            await writer.drain()
    except (ConnectionError, BrokenPipeError):
        pass
    finally:
        try:
            if writer.can_write_eof():
                writer.write_eof()
        except (OSError, RuntimeError):
            pass


async def handle(client_r, client_w, cmd):
    # Browsers pre-open idle sockets. iked's callback server handles one
    # connection at a time and would block on an idle one while the real
    # request waits, so only connect through once the client sends data.
    try:
        first = await asyncio.wait_for(client_r.read(65536), timeout=120)
    except (asyncio.TimeoutError, ConnectionError):
        first = b""
    if not first:
        client_w.close()
        return
    proc = await asyncio.create_subprocess_exec(
        *cmd, stdin=asyncio.subprocess.PIPE, stdout=asyncio.subprocess.PIPE)
    print(f"saml-forward: request from {client_w.get_extra_info('peername')}: "
          f"{first.split(b' HTTP/')[0][:60].decode(errors='replace')}", flush=True)
    proc.stdin.write(first)
    await asyncio.gather(pump(client_r, proc.stdin), pump(proc.stdout, client_w))
    client_w.close()
    try:
        proc.kill()
    except ProcessLookupError:
        pass
    await proc.wait()


async def main():
    if len(sys.argv) < 4 or sys.argv[2] != "--":
        sys.exit(__doc__)
    port, cmd = int(sys.argv[1]), sys.argv[3:]
    servers = []
    for host, family in (("127.0.0.1", socket.AF_INET), ("::1", socket.AF_INET6)):
        try:
            servers.append(await asyncio.start_server(
                lambda r, w: handle(r, w, cmd), host, port, family=family))
        except OSError as e:
            print(f"saml-forward: cannot listen on [{host}]:{port}: {e}", file=sys.stderr)
    if not servers:
        sys.exit(1)
    print(f"saml-forward: listening on localhost:{port}", flush=True)
    await asyncio.gather(*(s.serve_forever() for s in servers))


if __name__ == "__main__":
    try:
        asyncio.run(main())
    except KeyboardInterrupt:
        pass
