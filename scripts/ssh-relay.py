#!/usr/bin/env python3
"""TCP-туннель до sshd через локальный прокси (HTTP CONNECT или SOCKS5).

Провайдеры в РФ нередко рубят 22-й порт: ssh падает с «Connection refused»,
хотя через прокси тот же sshd отвечает. Скрипт запускается как ProxyCommand:

    KT_SSH_PROXY=socks5://127.0.0.1:10808 \
      ssh -o "ProxyCommand=python3 scripts/ssh-relay.py %h %p" nikita@example.org

Поддерживаются `socks5://host:port` и `http://host:port` (CONNECT).
Без KT_SSH_PROXY берётся http://127.0.0.1:10809.
"""
import os
import socket
import sys
import threading

TIMEOUT = 20


def log(message):
    """Диагностика: KT_SSH_RELAY_DEBUG=1 печатает шаги в stderr."""
    if os.environ.get("KT_SSH_RELAY_DEBUG"):
        sys.stderr.write("ssh-relay: %s\n" % message)
        sys.stderr.flush()


def proxy_address():
    """Возвращает (схема, хост, порт) прокси из KT_SSH_PROXY."""
    raw = os.environ.get("KT_SSH_PROXY", "")
    if not raw:
        host = os.environ.get("KT_SSH_PROXY_HOST", "127.0.0.1")
        port = os.environ.get("KT_SSH_PROXY_PORT", "10809")
        return "http", host, int(port)
    scheme, _, rest = raw.partition("://")
    if not rest:  # указали просто «host:port» — считаем это http-прокси
        scheme, rest = "http", scheme
    rest = rest.rstrip("/")
    host, _, port = rest.partition(":")
    return scheme.lower(), host or "127.0.0.1", int(port or 8080)


def recv_exact(sock, count, what):
    data = b""
    while len(data) < count:
        chunk = sock.recv(count - len(data))
        if not chunk:
            sys.exit("ssh-relay: прокси закрыл соединение (%s)" % what)
        data += chunk
    return data


def dial_http(host, port, phost, pport):
    sock = socket.create_connection((phost, pport), timeout=TIMEOUT)
    request = "CONNECT %s:%d HTTP/1.1\r\nHost: %s:%d\r\n\r\n" % (host, port, host, port)
    sock.sendall(request.encode("ascii"))
    head = b""
    while b"\r\n\r\n" not in head:
        chunk = sock.recv(1)
        if not chunk:
            sys.exit("ssh-relay: прокси %s:%d закрыл соединение" % (phost, pport))
        head += chunk
    status = head.split(b"\r\n", 1)[0]
    if b" 200" not in status:
        sys.exit("ssh-relay: прокси ответил %s" % status.decode("utf-8", "replace"))
    return sock


def dial_socks5(host, port, phost, pport):
    sock = socket.create_connection((phost, pport), timeout=TIMEOUT)
    sock.sendall(b"\x05\x01\x00")  # пятёрка, один способ, «без пароля»
    hello = recv_exact(sock, 2, "приветствие SOCKS5")
    if hello[0] != 5 or hello[1] != 0:
        sys.exit("ssh-relay: SOCKS5 %s:%d не принял соединение без пароля" % (phost, pport))
    name = host.encode("idna") if not host.replace(".", "").isdigit() else None
    if name is None:
        request = b"\x05\x01\x00\x01" + socket.inet_aton(host)
    else:
        request = b"\x05\x01\x00\x03" + bytes([len(name)]) + name
    sock.sendall(request + port.to_bytes(2, "big"))
    reply = recv_exact(sock, 4, "ответ SOCKS5")
    if reply[1] != 0:
        errors = {1: "общий сбой", 2: "правило запрещает", 3: "сеть недоступна",
                  4: "хост недоступен", 5: "отказ", 6: "таймаут", 7: "команда не поддержана",
                  8: "адрес не поддержан"}
        sys.exit("ssh-relay: SOCKS5 не пустил: %s" % errors.get(reply[1], reply[1]))
    atyp = reply[3]
    if atyp == 1:
        recv_exact(sock, 4, "адрес SOCKS5")
    elif atyp == 3:
        length = recv_exact(sock, 1, "адрес SOCKS5")[0]
        recv_exact(sock, length, "адрес SOCKS5")
    elif atyp == 4:
        recv_exact(sock, 16, "адрес SOCKS5")
    recv_exact(sock, 2, "порт SOCKS5")
    return sock


def dial(host, port):
    scheme, phost, pport = proxy_address()
    if scheme in ("socks5", "socks5h", "socks"):
        sock = dial_socks5(host, port, phost, pport)
    else:
        sock = dial_http(host, port, phost, pport)
    sock.settimeout(None)
    return sock


def pump(src, dst, side):
    """Перекачивает байты из src в dst, пока не кончится src.

    src — сокет или буфер stdin (у него нет recv), dst — сокет или буфер stdout.
    Конец stdin закрывает только запись в сокет, иначе оборвался бы сеанс.
    """
    reader = getattr(src, "read1", None)
    total = 0
    try:
        while True:
            data = reader(65536) if reader else src.recv(65536)
            if not data:
                log("%s: конец потока после %d байт" % (side, total))
                break
            total += len(data)
            sender = getattr(dst, "sendall", None)
            if sender:
                sender(data)
            else:
                dst.write(data)
                dst.flush()
    except (OSError, ValueError, AttributeError) as err:
        log("%s: обрыв после %d байт (%s)" % (side, total, err))
    finally:
        try:
            if hasattr(dst, "shutdown"):
                dst.shutdown(socket.SHUT_WR)
            else:
                dst.close()
        except OSError:
            pass


def main():
    if len(sys.argv) < 3:
        sys.exit("ssh-relay: нужно указать хост и порт (ssh подставит %h и %p)")
    log("подключаюсь к %s:%s через %s" % (sys.argv[1], sys.argv[2], proxy_address()))
    sock = dial(sys.argv[1], int(sys.argv[2]))
    log("канал открыт")
    threading.Thread(target=pump, args=(sys.stdin.buffer, sock, "ssh→sshd"), daemon=True).start()
    pump(sock, sys.stdout.buffer, "sshd→ssh")
    log("канал закрыт")


if __name__ == "__main__":
    main()
