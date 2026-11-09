#!/usr/bin/env python3
"""
HA-Panel — backend del panel web de orquestación (VPS sehuacho).
Expone estado de: MySQL Orchestrator, HAProxy, servicios systemd, WireGuard,
recursos del VPS y eventos; y acciones: enable/disable de servers HAProxy
y failover manual (invoca failover-manual.sh). También el estado y el redeploy de
Kipu (app.sehuacho.com), que corre aparte con Postgres (ver sección Kipu), y de
RadioClub25 (radioclub25peru.com, ver sección RadioClub25).

Stdlib-only (Python 3.8). Escucha SOLO en 127.0.0.1:9099 — el acceso externo
pasa por HAProxy (TLS + basic auth, path /panel-ha).
"""
import json
import os
import re
import socket
import subprocess
import threading
import time
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

BIND = ("127.0.0.1", 9099)
PREFIX = "/panel-ha"
HAPROXY_SOCK = "/run/haproxy-master.sock"
ORCH_API = "http://127.0.0.1:3000/api"
CLUSTER = "10.10.0.2:3308"
CURRENT_MASTER_FILE = "/var/lib/orchestrator/current_master"
EVENTS_LOG = "/var/log/orchestrator/events.log"
FAILOVER_SH = "/usr/local/bin/failover-manual.sh"
PANEL_LOG = "/var/log/orchestrator/panel.log"
FRONTEND = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "frontend", "index.html")

SERVICES = ["haproxy", "orchestrator", "sehuacho-bot", "fail2ban", "ssh"]
NODE_NAMES = {"10.10.0.2": "Nodo A", "10.10.0.3": "Nodo B", "10.10.0.4": "Nodo C"}

# Estado del failover en curso (solo uno a la vez)
_failover = {"running": False, "node": None, "started": None, "output": "", "rc": None}
_failover_lock = threading.Lock()

# Deploy: el panel lo dispara SOBRE EL MASTER actual (deploy.sh se auto-protege igual).
# IP del master -> (usuario SSH, ruta del repo). SSH del VPS como root a los nodos.
DEPLOY_NODES = {
    "10.10.0.2": ("lima", "/home/lima/sehuacho/restaurante-saas"),
    "10.10.0.3": ("oyon", "/home/oyon/sehuacho/restaurante-saas"),
    "10.10.0.4": ("server3", "/home/server3/restaurante-saas"),
}
SSH_KEY = "/root/.ssh/id_ed25519"
_deploy = {"running": False, "target": None, "started": None, "output": "", "rc": None}
_deploy_lock = threading.Lock()

# ── nodos del cluster (para recursos + desacoplar/acoplar) ─────────────────────
# IP -> (usuario SSH, letra, puerto SSH, server_id MySQL). Puerto 22 para C, 1522 resto.
NODES = {
    "10.10.0.2": {"user": "lima", "letter": "A", "port": 1522, "server_id": 3},
    "10.10.0.3": {"user": "oyon", "letter": "B", "port": 1522, "server_id": 2},
    "10.10.0.4": {"user": "server3", "letter": "C", "port": 22, "server_id": 4},
}
LETTER_IP = {v["letter"]: ip for ip, v in NODES.items()}
DB_CONTAINER = "sehuacho_db"


def _load_secret(key, default=""):
    """Secreto desde os.environ (systemd EnvironmentFile) o el env file. Ver .env.example."""
    v = os.environ.get(key)
    if v:
        return v
    try:
        with open(os.environ.get("ORCHESTRATOR_ENV", "/etc/sehuacho/orchestrator.env")) as f:
            for line in f:
                line = line.strip()
                if line and not line.startswith("#") and "=" in line:
                    k, val = line.split("=", 1)
                    if k.strip() == key:
                        return val.strip().strip('"').strip("'")
    except OSError:
        pass
    return default


DB_ROOT_PW = _load_secret("MYSQL_ROOT_PW")
REPL_USER = "replicator"
REPL_PASS = _load_secret("REPL_PASS")
DECOUPLED_DIR = "/var/lib/orchestrator/decoupled"
NOTIFY_SH = "/usr/local/bin/orchestrator-notify.sh"

# Caché de recursos por nodo, refrescada por un hilo en background (no en el request).
_node_res = {"ts": 0, "nodes": []}
_node_res_lock = threading.Lock()

# Estado semi-sync del master (garantía anti-pérdida de datos), refrescado en background.
_semisync = {"ts": 0, "master_ip": None, "status_on": None, "clients": None, "ok": None}

# Operación de nodo en curso (desacoplar/acoplar): solo una a la vez.
_nodeop = {"running": False, "op": None, "node": None, "started": None, "output": "", "rc": None}
_nodeop_lock = threading.Lock()


def node_ssh(ip, remote_cmd, timeout=15):
    """Ejecuta un comando por SSH en un nodo del cluster. Devuelve (rc, salida)."""
    info = NODES.get(ip)
    if not info:
        return 1, "nodo desconocido: %s" % ip
    cmd = ["ssh", "-i", SSH_KEY, "-p", str(info["port"]), "-o", "BatchMode=yes",
           "-o", "StrictHostKeyChecking=no", "-o", "ConnectTimeout=6",
           "%s@%s" % (info["user"], ip), remote_cmd]
    try:
        r = subprocess.run(cmd, capture_output=True, text=True, timeout=timeout)
        return r.returncode, (r.stdout or "") + (r.stderr or "")
    except subprocess.TimeoutExpired:
        return 124, "timeout"
    except Exception as e:
        return 1, str(e)


def db_exec(ip, sql, timeout=30):
    """Corre SQL como root dentro del contenedor MySQL del nodo (silencia stderr de mysql)."""
    inner = "docker exec sehuacho_db sh -lc \"mysql -uroot -p%s -N -e '%s'\"" % (DB_ROOT_PW, sql)
    return node_ssh(ip, inner, timeout=timeout)


def decoupled_letters():
    try:
        return sorted(f for f in os.listdir(DECOUPLED_DIR) if f in LETTER_IP)
    except OSError:
        return []


def log(msg):
    line = "[%s] %s\n" % (time.strftime("%Y-%m-%d %H:%M:%S"), msg)
    try:
        with open(PANEL_LOG, "a") as f:
            f.write(line)
    except OSError:
        pass


# ── colectores ────────────────────────────────────────────────────────────────

def haproxy_cmd(cmd):
    """Envía un comando al socket maestro de HAProxy (@1 = worker actual)."""
    s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    s.settimeout(5)
    try:
        s.connect(HAPROXY_SOCK)
        s.sendall(("@1 %s\n" % cmd).encode())
        s.shutdown(socket.SHUT_WR)  # half-close: el CLI maestro es interactivo y no cierra solo
        buf = b""
        while True:
            chunk = s.recv(65536)
            if not chunk:
                break
            buf += chunk
        return buf.decode(errors="replace")
    finally:
        s.close()


def haproxy_stats():
    """Parsea 'show stat' CSV → lista de backends con sus servers."""
    raw = haproxy_cmd("show stat")
    lines = [l for l in raw.splitlines() if l and not l.startswith("#")]
    header = None
    for l in raw.splitlines():
        if l.startswith("# "):
            header = l[2:].split(",")
            break
    if not header:
        return []
    backends = {}
    for l in lines:
        f = dict(zip(header, l.split(",")))
        px, sv = f.get("pxname", ""), f.get("svname", "")
        if not px or px in ("stats",):
            continue
        if sv == "FRONTEND":
            continue
        b = backends.setdefault(px, {"name": px, "status": None, "servers": []})
        entry = {
            "name": sv,
            "status": f.get("status", ""),
            "addr": f.get("addr", ""),
            "is_backup": f.get("bck", "0") == "1",
            "check": f.get("check_status", ""),
            "downtime": f.get("downtime", ""),
        }
        if sv == "BACKEND":
            b["status"] = f.get("status", "")
        else:
            b["servers"].append(entry)
    return list(backends.values())


def orch_get(path, timeout=6):
    try:
        with urllib.request.urlopen("%s/%s" % (ORCH_API, path), timeout=timeout) as r:
            return json.load(r)
    except Exception as e:
        return {"error": str(e)}


def topology():
    # Seguir al clúster del master actual: tras un failover el nombre del clúster
    # cambia y CLUSTER (fijo) se queda con el nodo caído aislado.
    cm = current_master()
    if cm and ":" in cm:
        host, port = cm.strip().split(":", 1)
        data = orch_get("cluster/instance/%s/%s" % (host, port))
    else:
        data = orch_get("cluster/%s" % CLUSTER)
    if not isinstance(data, list):
        err = data.get("error", "orchestrator sin datos") if isinstance(data, dict) else "orchestrator sin datos"
        return {"error": err, "nodes": []}
    nodes = []
    for i in data:
        ip = i["Key"]["Hostname"]
        master = i["MasterKey"]["Hostname"]
        nodes.append({
            "ip": ip,
            "name": NODE_NAMES.get(ip, ip),
            "port": i["Key"]["Port"],
            "role": "replica" if master else "master",
            "master": master or None,
            "version": i.get("Version", ""),
            "read_only": i.get("ReadOnly", False),
            "lag": (i.get("ReplicationLagSeconds") or {}).get("Int64", 0) if master else None,
            "replication_ok": (
                (i.get("Slave_IO_Running") or i.get("Replica_IO_Running", False))
                and (i.get("Slave_SQL_Running") or i.get("Replica_SQL_Running", False))
            ) if master else None,
            "uptime": i.get("Uptime", 0),
        })
    return {"nodes": nodes}


def problems():
    data = orch_get("problems")
    # el orchestrator devuelve JSON null (→ None) cuando NO hay problemas
    if not isinstance(data, list):
        return []
    out = []
    for i in data:
        ip = i["Key"]["Hostname"]
        out.append({"node": NODE_NAMES.get(ip, ip), "ip": ip, "problems": i.get("Problems", [])})
    return out


def services_status():
    out = []
    for svc in SERVICES:
        try:
            r = subprocess.run(["systemctl", "is-active", svc], capture_output=True, text=True, timeout=5)
            out.append({"name": svc, "active": r.stdout.strip() == "active", "state": r.stdout.strip()})
        except Exception as e:
            out.append({"name": svc, "active": False, "state": str(e)})
    return out


def wireguard_peers():
    try:
        r = subprocess.run(["wg", "show", "wg0", "latest-handshakes"], capture_output=True, text=True, timeout=5)
        peers = []
        pubkey_ip = {}
        d = subprocess.run(["wg", "show", "wg0", "allowed-ips"], capture_output=True, text=True, timeout=5)
        for l in d.stdout.strip().splitlines():
            parts = l.split("\t")
            if len(parts) == 2:
                ip = parts[1].split("/")[0]
                pubkey_ip[parts[0]] = ip
        now = time.time()
        for l in r.stdout.strip().splitlines():
            parts = l.split("\t")
            if len(parts) != 2:
                continue
            ip = pubkey_ip.get(parts[0], "?")
            hs = int(parts[1])
            age = int(now - hs) if hs else None
            peers.append({
                "ip": ip,
                "name": NODE_NAMES.get(ip, ip),
                "handshake_age": age,
                "ok": age is not None and age < 300,
            })
        return sorted(peers, key=lambda p: p["ip"])
    except Exception as e:
        return [{"error": str(e)}]


def sys_resources():
    mem = {}
    try:
        with open("/proc/meminfo") as f:
            for l in f:
                k, v = l.split(":", 1)
                mem[k] = int(v.strip().split()[0])  # kB
    except OSError:
        pass
    try:
        import shutil
        du = shutil.disk_usage("/")
        disk = {"total_gb": round(du.total / 2**30, 1), "used_gb": round(du.used / 2**30, 1),
                "pct": round(du.used * 100 / du.total)}
    except Exception:
        disk = {}
    return {
        "load": list(os.getloadavg()),
        "mem_total_mb": round(mem.get("MemTotal", 0) / 1024),
        "mem_avail_mb": round(mem.get("MemAvailable", 0) / 1024),
        "swap_used_mb": round((mem.get("SwapTotal", 0) - mem.get("SwapFree", 0)) / 1024),
        "disk": disk,
        "uptime_s": int(float(open("/proc/uptime").read().split()[0])),
    }


def _parse_node_res(ip, out):
    """Parsea la salida del one-liner remoto en un dict de recursos."""
    res = {"ip": ip, "name": NODE_NAMES.get(ip, ip), "reachable": True,
           "load": [], "nproc": None, "mem_total_mb": None, "mem_avail_mb": None,
           "mem_pct": None, "swap_used_mb": None, "disk": {}, "uptime_s": None}
    mem = {}
    swap_total = swap_free = 0
    for l in out.splitlines():
        p = l.split()
        if not p:
            continue
        if p[0] in ("MemTotal:", "MemAvailable:") and len(p) >= 2:
            mem[p[0].rstrip(":")] = int(p[1])  # kB
        elif p[0] == "SwapTotal:" and len(p) >= 2:
            swap_total = int(p[1])
        elif p[0] == "SwapFree:" and len(p) >= 2:
            swap_free = int(p[1])
        elif p[0] == "DISK" and len(p) >= 4:
            total_kb, used_kb, pct = int(p[1]), int(p[2]), p[3].rstrip("%")
            res["disk"] = {"total_gb": round(total_kb / 2**20, 1),
                           "used_gb": round(used_kb / 2**20, 1), "pct": int(pct)}
        elif len(p) == 3 and all(_isfloat(x) for x in p):  # /proc/loadavg (3 primeros campos)
            res["load"] = [float(x) for x in p]
        elif len(p) == 1 and p[0].isdigit() and res["nproc"] is None:
            res["nproc"] = int(p[0])
        elif len(p) == 1 and _isfloat(p[0]):
            res["uptime_s"] = int(float(p[0]))
    if mem.get("MemTotal"):
        res["mem_total_mb"] = round(mem["MemTotal"] / 1024)
        res["mem_avail_mb"] = round(mem.get("MemAvailable", 0) / 1024)
        res["mem_pct"] = round((mem["MemTotal"] - mem.get("MemAvailable", 0)) * 100 / mem["MemTotal"])
    res["swap_used_mb"] = round((swap_total - swap_free) / 1024)
    return res


def _isfloat(s):
    try:
        float(s)
        return True
    except ValueError:
        return False


def _collect_one(ip, out_list, idx):
    cmd = ("nproc; grep -E 'MemTotal|MemAvailable|SwapTotal|SwapFree' /proc/meminfo; "
           "df -P / | awk 'NR==2{print \"DISK\",$2,$3,$5}'; "
           "awk '{print $1,$2,$3}' /proc/loadavg; cut -d' ' -f1 /proc/uptime")
    rc, out = node_ssh(ip, cmd, timeout=8)
    if rc == 0:
        out_list[idx] = _parse_node_res(ip, out)
    else:
        out_list[idx] = {"ip": ip, "name": NODE_NAMES.get(ip, ip), "reachable": False,
                         "error": out.strip()[:120]}


def node_resources_collect():
    """Sondea los 3 nodos en paralelo y actualiza la caché global."""
    ips = list(NODES.keys())
    results = [None] * len(ips)
    threads = [threading.Thread(target=_collect_one, args=(ip, results, i))
               for i, ip in enumerate(ips)]
    for t in threads:
        t.start()
    for t in threads:
        t.join(timeout=10)
    with _node_res_lock:
        _node_res["ts"] = time.time()
        _node_res["nodes"] = [r for r in results if r]


def semisync_collect():
    """Sondea el estado semi-sync (lado source) del master actual y actualiza la caché."""
    mip = master_ip_now()
    status_on, clients = None, None
    if mip:
        rc, out = node_ssh(mip, "docker exec sehuacho_db sh -lc \"mysql -uroot -p%s -N -e "
                           "'SHOW GLOBAL STATUS' 2>/dev/null | grep -E "
                           "'Rpl_semi_sync_source_(status|clients)'\"" % DB_ROOT_PW, timeout=8)
        if rc == 0:
            for l in out.splitlines():
                p = l.split()
                if len(p) < 2:
                    continue
                if p[0] == "Rpl_semi_sync_source_status":
                    status_on = (p[1].upper() == "ON")
                elif p[0] == "Rpl_semi_sync_source_clients":
                    try:
                        clients = int(p[1])
                    except ValueError:
                        pass
    ok = bool(status_on) and (clients or 0) >= 1 if mip else None
    with _node_res_lock:
        _semisync.update(ts=time.time(), master_ip=mip, status_on=status_on, clients=clients, ok=ok)


def node_resources_loop():
    while True:
        try:
            node_resources_collect()
        except Exception as e:
            log("node_resources error: %s" % e)
        try:
            semisync_collect()
        except Exception as e:
            log("semisync error: %s" % e)
        time.sleep(20)


def _node_res_snapshot():
    with _node_res_lock:
        return {"ts": _node_res["ts"], "nodes": list(_node_res["nodes"])}


def _semisync_snapshot():
    with _node_res_lock:
        return dict(_semisync)


def tail_file(path, lines=60):
    try:
        r = subprocess.run(["tail", "-n", str(lines), path], capture_output=True, text=True, timeout=5)
        return r.stdout
    except Exception as e:
        return "error: %s" % e


def current_master():
    try:
        with open(CURRENT_MASTER_FILE) as f:
            return f.read().strip()
    except OSError:
        return None


# ── acciones ──────────────────────────────────────────────────────────────────

VALID_ACTION = {"enable", "disable"}
NAME_RE = re.compile(r"^[A-Za-z0-9_.-]+$")


def haproxy_server_action(backend, server, action):
    if action not in VALID_ACTION or not NAME_RE.match(backend) or not NAME_RE.match(server):
        return {"ok": False, "error": "parámetros inválidos"}
    out = haproxy_cmd("%s server %s/%s" % (action, backend, server)).strip()
    log("haproxy %s server %s/%s -> %r" % (action, backend, server, out))
    return {"ok": out == "", "output": out}


def start_failover(node):
    if node not in ("A", "B", "C"):
        return {"ok": False, "error": "nodo inválido"}
    with _failover_lock:
        if _failover["running"]:
            return {"ok": False, "error": "ya hay un failover en curso"}
        _failover.update(running=True, node=node, started=time.time(), output="", rc=None)
    log("FAILOVER MANUAL iniciado desde panel -> nodo %s" % node)

    def run():
        try:
            p = subprocess.Popen([FAILOVER_SH, node, "--force"], stdout=subprocess.PIPE,
                                 stderr=subprocess.STDOUT, text=True)
            for line in p.stdout:
                with _failover_lock:
                    _failover["output"] += line
            p.wait(timeout=300)
            rc = p.returncode
        except Exception as e:
            with _failover_lock:
                _failover["output"] += "\nERROR: %s" % e
            rc = -1
        with _failover_lock:
            _failover["running"] = False
            _failover["rc"] = rc
        log("FAILOVER nodo %s terminado rc=%s" % (node, rc))

    threading.Thread(target=run, daemon=True).start()
    return {"ok": True}


def start_deploy(check=False):
    # El deploy SOLO debe correr en el master. Lo detectamos por la topología del
    # orchestrator y hacemos SSH a ese nodo; deploy.sh además se auto-protege
    # (aborta si el nodo no es master / hay split-brain).
    # check=True corre 'deploy.sh --check' (valida pre-flight sin tocar producción).
    topo = topology()
    master = next((n for n in topo.get("nodes", []) if n.get("role") == "master"), None)
    if not master:
        return {"ok": False, "error": "no pude determinar el master (¿orchestrator caído?)"}
    ip = master["ip"]
    if ip not in DEPLOY_NODES:
        return {"ok": False, "error": "el master %s no está en el mapa de deploy" % ip}
    user, repo = DEPLOY_NODES[ip]
    with _deploy_lock:
        if _deploy["running"]:
            return {"ok": False, "error": "ya hay un deploy/verificación en curso"}
        _deploy.update(running=True, target="%s%s (%s)" % ("[check] " if check else "", master["name"], ip),
                       started=time.time(), output="", rc=None)
    log("%s iniciado desde panel -> master %s (%s@%s)" %
        ("CHECK" if check else "DEPLOY", master["name"], user, ip))

    script = "cd %s && bash deploy.sh %s2>&1" % (repo, "--check " if check else "")
    port = "22" if ip == "10.10.0.4" else "1522"

    def run():
        cmd = ["ssh", "-i", SSH_KEY, "-p", port, "-o", "BatchMode=yes",
               "-o", "StrictHostKeyChecking=no", "-o", "ConnectTimeout=10",
               "%s@%s" % (user, ip), script]
        try:
            p = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
            for line in p.stdout:
                with _deploy_lock:
                    _deploy["output"] += line
            p.wait(timeout=1200)
            rc = p.returncode
        except Exception as e:
            with _deploy_lock:
                _deploy["output"] += "\nERROR: %s" % e
            rc = -1
        with _deploy_lock:
            _deploy["running"] = False
            _deploy["rc"] = rc
        log("DEPLOY terminado rc=%s" % rc)

    threading.Thread(target=run, daemon=True).start()
    return {"ok": True, "target": _deploy["target"]}


# ── comprobación post-deploy (solo lectura) ─────────────────────────────────────
# Confirma que el deploy dejó cada nodo como debe: su override propio intacto
# (server-id / report-host únicos: si un nodo hereda el de otro desaparece de la
# topología), el mismo commit que el master, repo limpio, replicación sana y /up.
OVERRIDE_REL = "backend/docker-compose.override.yml"
_postcheck = {"running": False, "started": None, "finished": None, "checks": [], "ok": None}
_postcheck_lock = threading.Lock()


def _postcheck_probe(ip):
    """Una sola conexión SSH por nodo; devuelve dict KEY -> valor."""
    repo = DEPLOY_NODES[ip][1]
    script = (
        "cd %(repo)s 2>/dev/null || { echo REPO=MISSING; exit 0; }; "
        "echo HEAD=$(git rev-parse --short HEAD); "
        "echo DIRTY=$(git status --porcelain --untracked-files=no | wc -l); "
        "f=%(ov)s; if [ -f $f ]; then "
        "  echo OV_SID=$(grep -oE -- '--server-id=[0-9]+' $f | cut -d= -f2); "
        "  echo OV_RH=$(grep -oE -- '--report-host=[0-9.]+' $f | cut -d= -f2); "
        "  echo OV_TRACKED=$(git ls-files $f | wc -l); "
        "else echo OV_SID=MISSING; fi; "
        "docker exec sehuacho_db mysql -uroot -p%(pw)s -N -e "
        "\"SELECT CONCAT('SID=',@@server_id); SELECT CONCAT('RH=',IFNULL(@@report_host,'')); "
        "SELECT CONCAT('RO=',@@read_only); "
        "SELECT CONCAT('RH_PERSIST=',VARIABLE_VALUE) FROM performance_schema.persisted_variables "
        "WHERE VARIABLE_NAME='report_host'\" 2>/dev/null; "
        "docker exec sehuacho_db mysql -uroot -p%(pw)s -e 'SHOW REPLICA STATUS\\G' 2>/dev/null "
        "| grep -E '^ *(Source_Host|Replica_IO_Running|Replica_SQL_Running|Seconds_Behind_Source|Last_(IO|SQL)_Error):' "
        "| sed -E 's/^ *([A-Za-z_]+): ?/\\1=/'; "
        "echo UP=$(curl -s -o /dev/null -w '%%{http_code}' --max-time 8 http://localhost:8080/up || echo 000)"
    ) % {"repo": repo, "ov": OVERRIDE_REL, "pw": DB_ROOT_PW}
    rc, out = node_ssh(ip, script, timeout=40)
    d = {"_rc": rc}
    if rc == 255 or rc == 124:
        d["_err"] = out.strip()[-200:] or "sin conexión SSH"
        return d
    for line in out.splitlines():
        if "=" in line:
            k, v = line.split("=", 1)
            d.setdefault(k.strip(), v.strip())
    return d


def run_postcheck():
    checks = []

    def add(node, name, status, detail=""):
        checks.append({"node": node, "check": name, "status": status, "detail": detail})

    mip = master_ip_now()
    decoupled = {LETTER_IP[l] for l in decoupled_letters()}

    # Sondeo en paralelo (una conexión por nodo).
    probes = {}
    def worker(ip):
        probes[ip] = _postcheck_probe(ip)
    threads = [threading.Thread(target=worker, args=(ip,), daemon=True) for ip in NODES]
    for t in threads:
        t.start()
    for t in threads:
        t.join(timeout=50)

    master_head = (probes.get(mip) or {}).get("HEAD") if mip else None
    if not mip:
        add("Cluster", "Master", "bad", "no pude determinar el master")

    for ip, info in NODES.items():
        name = NODE_NAMES.get(ip, ip)
        d = probes.get(ip) or {"_err": "sin respuesta"}
        is_master = (ip == mip)
        if "_err" in d:
            add(name, "Conexión", "bad", d["_err"])
            continue
        if d.get("REPO") == "MISSING":
            add(name, "Repo", "bad", "no existe %s" % DEPLOY_NODES[ip][1])
            continue

        # Código
        head = d.get("HEAD", "?")
        if master_head and head != master_head:
            add(name, "Commit", "bad", "%s ≠ master %s" % (head, master_head))
        else:
            add(name, "Commit", "ok", head)
        add(name, "Repo limpio", "ok" if d.get("DIRTY") == "0" else "bad",
            "sin cambios trackeados" if d.get("DIRTY") == "0" else "%s archivo(s) trackeados modificados" % d.get("DIRTY"))

        # Override propio del nodo
        exp_sid, exp_rh = str(info["server_id"]), ip
        if d.get("OV_SID") == "MISSING":
            add(name, "Override", "bad", "falta %s (se perdió en el pull)" % OVERRIDE_REL)
        elif d.get("OV_SID") != exp_sid or d.get("OV_RH") != exp_rh:
            add(name, "Override", "bad", "server-id=%s report-host=%s (esperado %s / %s): tiene la config de otro nodo"
                % (d.get("OV_SID") or "?", d.get("OV_RH") or "?", exp_sid, exp_rh))
        else:
            add(name, "Override", "ok", "server-id=%s report-host=%s%s" % (
                exp_sid, exp_rh, " · ⚠ aún trackeado en git" if d.get("OV_TRACKED") not in (None, "0") else ""))

        # MySQL en ejecución
        sid, rh = d.get("SID"), d.get("RH", "")
        if sid is None:
            add(name, "MySQL", "bad", "no responde")
        else:
            add(name, "server-id", "ok" if sid == exp_sid else "bad",
                sid if sid == exp_sid else "%s (esperado %s)" % (sid, exp_sid))
            if rh == exp_rh:
                add(name, "report-host", "ok", rh)
            elif not rh and d.get("RH_PERSIST") == exp_rh:
                add(name, "report-host", "warn", "persistido %s, aplica en el próximo reinicio" % exp_rh)
            else:
                add(name, "report-host", "bad", "%s (esperado %s)" % (rh or "vacío", exp_rh))

            ro = d.get("RO")
            if is_master:
                add(name, "Rol", "ok" if ro == "0" else "bad", "master escribible" if ro == "0" else "master en read_only")
            elif ip in decoupled:
                add(name, "Rol", "warn", "nodo desacoplado (aislado): se omite la replicación")
            else:
                add(name, "Rol", "ok" if ro == "1" else "bad", "réplica read-only" if ro == "1" else "réplica ESCRIBIBLE (riesgo split-brain)")
                io, sql = d.get("Replica_IO_Running"), d.get("Replica_SQL_Running")
                lag = d.get("Seconds_Behind_Source", "NULL")
                src = d.get("Source_Host", "")
                if io == "Yes" and sql == "Yes":
                    st = "ok" if lag.isdigit() and int(lag) <= 30 else "warn"
                    add(name, "Replicación", st, "IO/SQL Yes · lag %ss · de %s" % (lag, src))
                    if mip and src and src != mip:
                        add(name, "Replicación", "bad", "replica de %s, no del master %s" % (src, mip))
                else:
                    err = d.get("Last_IO_Error") or d.get("Last_SQL_Error") or ""
                    add(name, "Replicación", "bad", "IO=%s SQL=%s %s" % (io or "?", sql or "?", err[:160]))

        # App
        up = d.get("UP", "000")
        add(name, "App /up", "ok" if up == "200" else "bad", "HTTP %s" % up)

    # Visión del Orchestrator: los 3 nodos, cada uno con su propia dirección.
    topo = topology()
    seen = [n["ip"] for n in topo.get("nodes", [])]
    faltan = [NODE_NAMES[ip] for ip in NODES if ip not in seen and ip not in decoupled]
    if topo.get("error"):
        add("Cluster", "Orchestrator", "bad", topo["error"])
    elif faltan:
        add("Cluster", "Orchestrator", "bad", "no ve: %s" % ", ".join(faltan))
    else:
        add("Cluster", "Orchestrator", "ok", "ve %s" % ", ".join(NODE_NAMES.get(i, i) for i in seen))
    sids = [probes[ip].get("SID") for ip in NODES if probes.get(ip, {}).get("SID")]
    dup = sorted({s for s in sids if sids.count(s) > 1})
    add("Cluster", "server-id únicos", "bad" if dup else "ok",
        "repetidos: %s" % ", ".join(dup) if dup else ", ".join(sids))
    ss = _semisync_snapshot()
    add("Cluster", "Semi-sync", "ok" if ss.get("ok") else "warn",
        "ON · %s réplica(s)" % ss.get("clients") if ss.get("ok") else "sin garantía (status=%s clients=%s)" % (ss.get("status_on"), ss.get("clients")))
    return checks


def start_postcheck():
    with _postcheck_lock:
        if _postcheck["running"]:
            return {"ok": False, "error": "ya hay una comprobación en curso"}
        _postcheck.update(running=True, started=time.time(), finished=None, checks=[], ok=None)
    log("POST-DEPLOY check iniciado desde panel")

    def run():
        try:
            checks = run_postcheck()
        except Exception as e:
            checks = [{"node": "Panel", "check": "Error", "status": "bad", "detail": str(e)}]
        ok = not any(c["status"] == "bad" for c in checks)
        with _postcheck_lock:
            _postcheck.update(running=False, finished=time.time(), checks=checks, ok=ok)
        bad = [c for c in checks if c["status"] == "bad"]
        log("POST-DEPLOY check terminado ok=%s %s" % (ok, "; ".join("%s %s: %s" % (c["node"], c["check"], c["detail"]) for c in bad)))

    threading.Thread(target=run, daemon=True).start()
    return {"ok": True}


# ── desacoplar / acoplar nodos ──────────────────────────────────────────────────

def master_ip_now():
    """IP del master actual: current_master (fuente de verdad manual) -> topología."""
    cm = current_master()
    if cm:
        ip = cm.split(":")[0].strip()
        if ip in NODES:
            return ip
    for n in topology().get("nodes", []):
        if n.get("role") == "master":
            return n["ip"]
    return None


def haproxy_node_servers(letter):
    """(backend, server) de HAProxy que corresponden al nodo (server 'nodo_<letra>')."""
    target = "nodo_%s" % letter.lower()
    out = []
    for b in haproxy_stats():
        for s in b.get("servers", []):
            if s["name"] == target:
                out.append((b["name"], s["name"]))
    return out


def _nodeop_emit(msg):
    with _nodeop_lock:
        _nodeop["output"] += msg + "\n"
    log("[nodeop] " + msg.replace("\n", " "))


def notify(kind, msg):
    try:
        subprocess.run([NOTIFY_SH, kind, msg], timeout=15,
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    except Exception:
        pass


def _start_nodeop(op, node):
    """Guardas comunes + arranque del estado. Devuelve (ip, err_dict|None)."""
    if node not in LETTER_IP:
        return None, {"ok": False, "error": "nodo inválido"}
    ip = LETTER_IP[node]
    mip = master_ip_now()
    if mip is None:
        return None, {"ok": False, "error": "no pude determinar el master (¿orchestrator caído?)"}
    if ip == mip:
        return None, {"ok": False, "error": "el Nodo %s es el MASTER actual; no se puede %s" % (node, op)}
    already = node in decoupled_letters()
    if op == "desacoplar" and already:
        return None, {"ok": False, "error": "el Nodo %s ya está desacoplado" % node}
    if op == "acoplar" and not already:
        return None, {"ok": False, "error": "el Nodo %s no está desacoplado" % node}
    with _nodeop_lock:
        if _nodeop["running"]:
            return None, {"ok": False, "error": "ya hay una operación de nodo en curso"}
        _nodeop.update(running=True, op=op, node=node, started=time.time(), output="", rc=None)
    return ip, None


def _finish_nodeop(rc):
    with _nodeop_lock:
        _nodeop["running"] = False
        _nodeop["rc"] = rc


def start_decouple(node):
    ip, err = _start_nodeop("desacoplar", node)
    if err:
        return err

    def run():
        rc = 0
        try:
            _nodeop_emit(">> Desacoplando Nodo %s (%s)" % (node, ip))
            os.makedirs(DECOUPLED_DIR, exist_ok=True)
            with open(os.path.join(DECOUPLED_DIR, node), "w") as f:
                f.write("%s desacoplado manualmente desde el panel\n" % time.strftime("%Y-%m-%d %H:%M:%S"))
            _nodeop_emit("   flag de desacople creado")

            _nodeop_emit(">> HAProxy: sacando de rotación (MAINT)")
            servers = haproxy_node_servers(node)
            if not servers:
                _nodeop_emit("   (aviso) no hay servers nodo_%s en HAProxy" % node.lower())
            for bk, sv in servers:
                r = haproxy_server_action(bk, sv, "disable")
                _nodeop_emit("   disable %s/%s -> %s" % (bk, sv, "ok" if r.get("ok") else r))

            _nodeop_emit(">> MySQL: parando replicación y forzando read-only")
            src, out = db_exec(ip, "STOP REPLICA; SET GLOBAL super_read_only=ON; SET GLOBAL read_only=ON;")
            if src != 0:
                # fallback sintaxis antigua
                src, out = db_exec(ip, "STOP SLAVE; SET GLOBAL super_read_only=ON; SET GLOBAL read_only=ON;")
            _nodeop_emit("   %s" % ("replicación detenida, nodo read-only" if src == 0 else "ERROR: %s" % out.strip()))
            if src != 0:
                rc = 1
            _nodeop_emit("== Nodo %s DESACOPLADO ==" % node)
            notify("alert", "Nodo %s desacoplado manualmente desde el panel (fuera de HAProxy, replicación detenida)." % node)
        except Exception as e:
            _nodeop_emit("ERROR: %s" % e)
            rc = -1
        _finish_nodeop(rc)

    threading.Thread(target=run, daemon=True).start()
    return {"ok": True}


def start_couple(node):
    ip, err = _start_nodeop("acoplar", node)
    if err:
        return err
    mip = master_ip_now()
    server_id = NODES[ip]["server_id"]

    def run():
        rc = 0
        try:
            _nodeop_emit(">> ACOPLAR Nodo %s (%s) — reclonando desde master %s" % (node, ip, mip))

            # Limpia estado local Y baja super_read_only para que la carga del dump
            # pueda escribir (una réplica/nodo desacoplado está super_read_only=ON).
            # El CHANGE REPLICATION SOURCE final vuelve a poner read_only=ON.
            _nodeop_emit(">> MySQL: limpiando estado local + habilitando escritura para el reseed")
            src, out = db_exec(ip, "STOP REPLICA; RESET REPLICA ALL; RESET MASTER; SET GLOBAL super_read_only=OFF; SET GLOBAL read_only=OFF;")
            if src != 0:
                src, out = db_exec(ip, "STOP SLAVE; RESET SLAVE ALL; RESET MASTER; SET GLOBAL super_read_only=OFF; SET GLOBAL read_only=OFF;")
            if src != 0:
                _nodeop_emit("ERROR limpiando estado: %s" % out.strip())
                return _finish_nodeop(1)

            _nodeop_emit(">> Reseed: mysqldump del master %s -> Nodo %s (puede tardar)" % (mip, node))
            rc = _reseed_from_master(mip, ip)
            if rc != 0:
                _nodeop_emit("ERROR: el reseed falló (rc=%s). El nodo queda SIN replicación; revisar." % rc)
                notify("alert", "Acople de Nodo %s FALLÓ durante el reseed. Intervención manual." % node)
                return _finish_nodeop(rc)

            _nodeop_emit(">> MySQL: configurando replicación GTID hacia el master")
            chg = ("CHANGE REPLICATION SOURCE TO SOURCE_HOST=\\\"%s\\\", SOURCE_PORT=3308, "
                   "SOURCE_USER=\\\"%s\\\", SOURCE_PASSWORD=\\\"%s\\\", SOURCE_AUTO_POSITION=1, "
                   "GET_SOURCE_PUBLIC_KEY=1; SET GLOBAL server_id=%s; "
                   "SET GLOBAL super_read_only=ON; SET GLOBAL read_only=ON; START REPLICA;"
                   % (mip, REPL_USER, REPL_PASS, server_id))
            src, out = db_exec(ip, chg)
            if src != 0:
                _nodeop_emit("ERROR configurando replicación: %s" % out.strip())
                return _finish_nodeop(1)
            _nodeop_emit("   replicación iniciada (AUTO_POSITION)")

            _nodeop_emit(">> Quitando flag de desacople y reactivando en HAProxy")
            try:
                os.remove(os.path.join(DECOUPLED_DIR, node))
            except OSError:
                pass
            for bk, sv in haproxy_node_servers(node):
                r = haproxy_server_action(bk, sv, "enable")
                _nodeop_emit("   enable %s/%s -> %s" % (bk, sv, "ok" if r.get("ok") else r))

            _nodeop_emit(">> Forzando redescubrimiento en el orchestrator")
            orch_get("discover/%s/3308" % ip)

            _nodeop_emit("== Nodo %s ACOPLADO como réplica de %s ==" % (node, mip))
            notify("alert", "Nodo %s reacoplado como réplica del master %s (reclonado)." % (node, mip))
        except Exception as e:
            _nodeop_emit("ERROR: %s" % e)
            rc = -1
        _finish_nodeop(rc)

    threading.Thread(target=run, daemon=True).start()
    return {"ok": True}


def _reseed_from_master(mip, ip):
    """mysqldump del master (por SSH) canalizado a mysql del nodo (por SSH). Devuelve rc."""
    minfo, ninfo = NODES[mip], NODES[ip]
    dbname = master_app_db(mip)
    if not dbname:
        _nodeop_emit("   ERROR: no pude determinar la BD de la app en el master")
        return 2
    _nodeop_emit("   BD a clonar: %s" % dbname)
    dump = ("docker exec %s mysqldump -uroot -p%s --single-transaction --set-gtid-purged=ON "
            "--routines --triggers --events --databases %s"
            % (DB_CONTAINER, DB_ROOT_PW, dbname))
    load = "docker exec -i %s mysql -uroot -p%s" % (DB_CONTAINER, DB_ROOT_PW)
    src_ssh = ["ssh", "-i", SSH_KEY, "-p", str(minfo["port"]), "-o", "BatchMode=yes",
               "-o", "StrictHostKeyChecking=no", "-o", "ConnectTimeout=6",
               "%s@%s" % (minfo["user"], mip), dump]
    dst_ssh = ["ssh", "-i", SSH_KEY, "-p", str(ninfo["port"]), "-o", "BatchMode=yes",
               "-o", "StrictHostKeyChecking=no", "-o", "ConnectTimeout=6",
               "%s@%s" % (ninfo["user"], ip), load]
    try:
        p1 = subprocess.Popen(src_ssh, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        p2 = subprocess.Popen(dst_ssh, stdin=p1.stdout, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        p1.stdout.close()  # deja que p1 reciba SIGPIPE si p2 muere
        out2, _ = p2.communicate(timeout=1800)
        err1 = p1.stderr.read().decode(errors="replace")
        p1.wait(timeout=10)
        if p1.returncode not in (0, None):
            _nodeop_emit("   mysqldump rc=%s: %s" % (p1.returncode, err1.strip()[:300]))
        if out2:
            tail = out2.decode(errors="replace").strip()
            if tail:
                _nodeop_emit("   carga: %s" % tail[:300])
        return p2.returncode or (p1.returncode or 0)
    except subprocess.TimeoutExpired:
        _nodeop_emit("   ERROR: timeout en el reseed (>30min)")
        return 124
    except Exception as e:
        _nodeop_emit("   ERROR reseed: %s" % e)
        return 1


def master_app_db(mip):
    """Nombre de la BD de negocio: DB_DATABASE del .env del master (si está en el mapa de deploy)."""
    repo = DEPLOY_NODES.get(mip, (None, None))[1]
    if repo:
        rc, out = node_ssh(mip, "grep -m1 '^DB_DATABASE=' %s/backend/.env | cut -d= -f2-" % repo, timeout=8)
        if rc == 0 and out.strip():
            val = out.strip().strip('"').strip("'")
            if re.match(r"^[A-Za-z0-9_]+$", val):
                return val
    return None


def start_nodeop(op, node):
    if op == "decouple":
        return start_decouple(node)
    if op == "couple":
        return start_couple(node)
    return {"ok": False, "error": "operación inválida"}


# ── Kipu (app.sehuacho.com) ────────────────────────────────────────────────────
# Kipu corre active-passive APARTE del legado (repo kipu, scripts/ha): Postgres con
# una principal y réplicas en streaming, sin orchestrator. Aquí solo se LEE su
# estado y se dispara su redeploy; nada de esto toca MySQL, el orchestrator ni los
# servers nodo_* del legado. Sus servers de HAProxy se llaman kipu_a/b/c.
KIPU_WEB_PORT = {"10.10.0.2": 8095, "10.10.0.3": 8097, "10.10.0.4": 8097}
# Nodos donde está el repo de Kipu y desde los que se puede redesplegar. Solo A:
# tras un failover de Kipu el redeploy se hace a mano (ver scripts/ha/README.md).
KIPU_REPO = {"10.10.0.2": "/home/lima/sehuacho/kipu"}
KIPU_PROBE = (
    "echo REC $(docker exec kipu_prod_db psql -U kipu -d kipu_prod -Atc 'select pg_is_in_recovery()' 2>/dev/null || echo x); "
    "echo API $(docker inspect -f '{{.State.Running}}' kipu_prod_api 2>/dev/null || echo false); "
    "docker exec kipu_prod_db psql -U kipu -d kipu_prod -AtF' ' "
    "-c \"select 'WAL', status from pg_stat_wal_receiver\" "
    "-c \"select 'REP', client_addr, state, coalesce(extract(epoch from replay_lag)::int, 0) from pg_stat_replication\" 2>/dev/null; true"
)
_kipu = {"ts": 0, "nodes": []}
# Orquestador de Kipu (scripts/ha/vps del repo kipu): failover automático,
# reconciliador y tolerancia a datos no críticos. El panel solo lo consulta y le
# pide acciones a través de su CLI; la lógica vive allí, no aquí.
KIPU_HA_BIN = "/usr/local/bin/kipu-ha"
KIPU_HA_ESTADO = "/var/lib/kipu-ha"
_kipu_op = {"running": False, "op": None, "started": None, "output": "", "rc": None}
_kipu_op_lock = threading.Lock()


def _kipu_auto():
    """Estado del orquestador leído de sus ficheros (barato: sin SSH)."""
    if not os.path.exists(KIPU_HA_BIN):
        return {"instalado": False}
    def leer(ruta):
        try:
            with open(ruta) as f:
                return f.read().strip()
        except OSError:
            return ""
    aislados = {}
    try:
        for letra in sorted(os.listdir(os.path.join(KIPU_HA_ESTADO, "aislados"))):
            aislados[letra] = leer(os.path.join(KIPU_HA_ESTADO, "aislados", letra))
    except OSError:
        pass
    return {"instalado": True, "pausado": os.path.exists(os.path.join(KIPU_HA_ESTADO, "pausado")),
            "principal": leer(os.path.join(KIPU_HA_ESTADO, "principal")), "aislados": aislados}


def start_kipu_op(accion, nodo):
    if accion not in ("pausar", "reanudar", "failover", "reintegrar"):
        return {"ok": False, "error": "acción inválida"}
    if accion in ("failover", "reintegrar") and nodo not in LETTER_IP:
        return {"ok": False, "error": "nodo inválido"}
    if not os.path.exists(KIPU_HA_BIN):
        return {"ok": False, "error": "el orquestador de Kipu no está instalado"}
    args = [KIPU_HA_BIN, accion] + ([nodo] if accion in ("failover", "reintegrar") else [])
    if accion in ("pausar", "reanudar"):
        r = subprocess.run(args, capture_output=True, text=True, timeout=30)
        log("KIPU-HA %s desde panel -> rc=%s" % (accion, r.returncode))
        return {"ok": r.returncode == 0, "output": (r.stdout + r.stderr).strip()}
    with _kipu_op_lock:
        if _kipu_op["running"]:
            return {"ok": False, "error": "ya hay una operación de Kipu HA en curso"}
        _kipu_op.update(running=True, op="%s %s" % (accion, nodo), started=time.time(), output="", rc=None)
    log("KIPU-HA %s %s desde panel" % (accion, nodo))

    def run():
        try:
            p = subprocess.Popen(args, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
            for line in p.stdout:
                if " kB " in line:
                    continue
                with _kipu_op_lock:
                    _kipu_op["output"] += line
            p.wait(timeout=1800)
            rc = p.returncode
        except Exception as e:
            with _kipu_op_lock:
                _kipu_op["output"] += "\nERROR: %s" % e
            rc = -1
        with _kipu_op_lock:
            _kipu_op["running"] = False
            _kipu_op["rc"] = rc
        log("KIPU-HA %s %s terminado rc=%s" % (accion, nodo, rc))
        threading.Thread(target=kipu_collect, daemon=True).start()

    threading.Thread(target=run, daemon=True).start()
    return {"ok": True, "op": _kipu_op["op"]}
_kipu_lock = threading.Lock()
_kipu_deploy = {"running": False, "target": None, "started": None, "output": "", "rc": None}
_kipu_deploy_lock = threading.Lock()


def _kipu_health(ip):
    """Código HTTP de /api/v1/health por el mismo camino que HAProxy (200 = base principal)."""
    try:
        with urllib.request.urlopen("http://%s:%s/api/v1/health" % (ip, KIPU_WEB_PORT[ip]), timeout=4) as r:
            return r.status
    except urllib.error.HTTPError as e:
        return e.code
    except Exception:
        return None


def _kipu_collect_one(ip, out_list, idx):
    res = {"ip": ip, "name": NODE_NAMES.get(ip, ip), "reachable": False, "role": "?",
           "api": False, "wal": None, "replicas": [], "health": None}
    rc, out = node_ssh(ip, KIPU_PROBE, timeout=10)
    if rc == 0:
        res["reachable"] = True
        for l in out.splitlines():
            p = l.split()
            if len(p) >= 2 and p[0] == "REC":
                res["role"] = {"f": "principal", "t": "replica"}.get(p[1], "sin_base")
            elif len(p) >= 2 and p[0] == "API":
                res["api"] = p[1] == "true"
            elif len(p) >= 2 and p[0] == "WAL":
                res["wal"] = p[1]
            elif len(p) >= 4 and p[0] == "REP":
                res["replicas"].append({"ip": p[1], "name": NODE_NAMES.get(p[1], p[1]),
                                        "state": p[2], "lag_s": int(p[3]) if p[3].isdigit() else None})
    else:
        res["error"] = out.strip()[:120]
    res["health"] = _kipu_health(ip)
    out_list[idx] = res


def kipu_collect():
    ips = list(KIPU_WEB_PORT.keys())
    results = [None] * len(ips)
    threads = [threading.Thread(target=_kipu_collect_one, args=(ip, results, i)) for i, ip in enumerate(ips)]
    for t in threads:
        t.start()
    for t in threads:
        t.join(timeout=15)
    with _kipu_lock:
        _kipu["ts"] = time.time()
        _kipu["nodes"] = [r for r in results if r]


def kipu_loop():
    while True:
        try:
            kipu_collect()
        except Exception as e:
            log("kipu status error: %s" % e)
        time.sleep(20)


def _kipu_snapshot():
    with _kipu_lock:
        snap = {"ts": _kipu["ts"], "nodes": list(_kipu["nodes"])}
    snap["auto"] = _kipu_auto()
    return snap


def start_kipu_deploy(check=False):
    # Se redespliega en el nodo cuya base es la PRINCIPAL de Kipu, recién sondeado
    # (no la caché): con dos principales (split-brain) o ninguna, no se toca nada.
    kipu_collect()
    principales = [n for n in _kipu_snapshot()["nodes"] if n.get("role") == "principal"]
    if len(principales) != 1:
        return {"ok": False, "error": "Kipu tiene %d bases principales; se esperaba 1. Revisar a mano." % len(principales)}
    ip = principales[0]["ip"]
    if ip not in KIPU_REPO:
        return {"ok": False, "error": "la principal de Kipu es %s (tras failover): redeploy a mano, ver scripts/ha/README.md"
                                      % NODE_NAMES.get(ip, ip)}
    repo = KIPU_REPO[ip]
    with _deploy_lock:
        legado_en_curso = _deploy["running"]
    if legado_en_curso and not check:
        return {"ok": False, "error": "hay un deploy del legado en curso en el master; espera a que termine"}
    with _kipu_deploy_lock:
        if _kipu_deploy["running"]:
            return {"ok": False, "error": "ya hay un redeploy/verificación de Kipu en curso"}
        _kipu_deploy.update(running=True, target="%s%s (%s)" % ("[check] " if check else "", NODE_NAMES.get(ip, ip), ip),
                            started=time.time(), output="", rc=None)
    log("KIPU %s iniciado desde panel -> %s" % ("CHECK" if check else "REDEPLOY", ip))

    if check:
        script = (
            "cd %s && echo '== Código que se desplegaría (copia de trabajo, sin git pull)' && "
            "git rev-parse --abbrev-ref HEAD && git log --oneline -1 && "
            "echo \"cambios sin commit: $(git status --short | wc -l)\" && "
            "echo '== Configuración' && "
            "(grep -qE '^JWT_SECRET=.+' .env && echo 'JWT_SECRET ok' || echo 'FALTA JWT_SECRET') && "
            "(grep -qE '^KIPU_REPL_PASSWORD=.+' .env && echo 'clave de replicación ok' || echo 'FALTA KIPU_REPL_PASSWORD') && "
            "echo '== Disco' && df -h / | tail -1 && "
            "echo '== Salud actual' && curl -s --max-time 4 http://127.0.0.1:8095/api/v1/health; echo && "
            "echo '== Scripts' && for f in scripts/banco/redeploy-banco.sh scripts/ha/*.sh; do bash -n \"$f\" || echo \"ERROR sintaxis $f\"; done && "
            "echo 'scripts OK'"
        ) % repo
    else:
        script = "cd %s && scripts/banco/redeploy-banco.sh 2>&1" % repo

    def run():
        info = NODES[ip]
        cmd = ["ssh", "-i", SSH_KEY, "-p", str(info["port"]), "-o", "BatchMode=yes",
               "-o", "StrictHostKeyChecking=no", "-o", "ConnectTimeout=10",
               "%s@%s" % (info["user"], ip), script]
        try:
            p = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
            for line in p.stdout:
                # Las barras de progreso de pg_basebackup no aportan nada al log.
                if " kB " in line:
                    continue
                with _kipu_deploy_lock:
                    _kipu_deploy["output"] += line
            p.wait(timeout=1800)
            rc = p.returncode
        except Exception as e:
            with _kipu_deploy_lock:
                _kipu_deploy["output"] += "\nERROR: %s" % e
            rc = -1
        with _kipu_deploy_lock:
            _kipu_deploy["running"] = False
            _kipu_deploy["rc"] = rc
        log("KIPU %s terminado rc=%s" % ("CHECK" if check else "REDEPLOY", rc))
        threading.Thread(target=kipu_collect, daemon=True).start()

    threading.Thread(target=run, daemon=True).start()
    return {"ok": True, "target": _kipu_deploy["target"]}


# ── RadioClub25 (radioclub25peru.com) ─────────────────────────────────────────
# Radio + TV del cliente RadioClub25 (repo radioclub25, scripts/ha). App Go con
# SQLite, Icecast y MediaMTX en Docker; un nginx "edge" por nodo en :8125 de su IP
# de WireGuard. A es la principal; B y C tienen el stack PARADO y reciben copia de
# data/ cada 5 min (cron en A). HAProxy: backend radioclub25_nodes, servers
# rc25_a/b/c (no nodo_*: el desacople del legado no debe arrastrar a la radio).
# Aquí solo se lee su estado y se disparan sus scripts; la lógica vive en el repo.
RC25_REPO = {"10.10.0.2": "/home/lima/sehuacho/radioclub25",
             "10.10.0.3": "/home/oyon/sehuacho/radioclub25",
             "10.10.0.4": "/home/server3/sehuacho/radioclub25"}
RC25_EDGE_PORT = 8125
RC25_PUBLIC = "https://radioclub25peru.com"
_rc25 = {"ts": 0, "nodes": [], "aire": None}
_rc25_lock = threading.Lock()
_rc25_op = {"running": False, "op": None, "started": None, "output": "", "rc": None}
_rc25_op_lock = threading.Lock()


def _rc25_probe(repo):
    return (
        "cd %s 2>/dev/null || { echo NOREPO; exit 0; }; "
        "echo RUN $(docker ps --filter label=com.docker.compose.project=radioclub25 --format '{{.Label \"com.docker.compose.service\"}}' | sort | tr '\\n' ','); "
        "echo SYNC $(cat data/.ha-sync 2>/dev/null || echo -); "
        "echo PROM $(cat data/.ha-principal 2>/dev/null || echo -); "
        "echo DBSZ $(du -sm data/radioclub25.db 2>/dev/null | cut -f1); "
        "echo UPSZ $(du -sm data/uploads 2>/dev/null | cut -f1); "
        "echo GIT $(git log --oneline -1 2>/dev/null | cut -c1-70)"
    ) % repo


def _rc25_http(url, timeout=4):
    try:
        with urllib.request.urlopen(url, timeout=timeout) as r:
            return r.status, r.read(200000)
    except urllib.error.HTTPError as e:
        return e.code, b""
    except Exception:
        return None, b""


def _rc25_collect_one(ip, out_list, idx):
    res = {"ip": ip, "name": NODE_NAMES.get(ip, ip), "reachable": False, "repo": False,
           "running": [], "sync": None, "promovido": None, "db_mb": None, "uploads_mb": None,
           "git": "", "health": None}
    rc, out = node_ssh(ip, _rc25_probe(RC25_REPO[ip]), timeout=12)
    if rc == 0:
        res["reachable"] = True
        res["repo"] = "NOREPO" not in out
        for l in out.splitlines():
            k, _, v = l.partition(" ")
            v = v.strip()
            if k == "RUN":
                res["running"] = [s for s in v.split(",") if s]
            elif k == "SYNC" and v != "-":
                res["sync"] = v
            elif k == "PROM" and v != "-":
                res["promovido"] = v
            elif k == "DBSZ" and v.isdigit():
                res["db_mb"] = int(v)
            elif k == "UPSZ" and v.isdigit():
                res["uploads_mb"] = int(v)
            elif k == "GIT":
                res["git"] = v
    else:
        res["error"] = out.strip()[:120]
    res["health"], _ = _rc25_http("http://%s:%s/healthz" % (ip, RC25_EDGE_PORT))
    out_list[idx] = res


def rc25_collect():
    ips = list(RC25_REPO)
    out = [None] * len(ips)
    th = [threading.Thread(target=_rc25_collect_one, args=(ip, out, i)) for i, ip in enumerate(ips)]
    for t in th:
        t.start()
    for t in th:
        t.join(20)
    # Estado al aire leído del nodo que atiende (el primero sano), por WireGuard.
    aire = None
    for n in out:
        if n and n.get("health") == 200:
            code, body = _rc25_http("http://%s:%s/api/status" % (n["ip"], RC25_EDGE_PORT))
            if code == 200:
                try:
                    st = json.loads(body)
                    aire = {"desde": n["name"], "tv": (st.get("tv") or {}).get("online"),
                            "estaciones": [{"label": s.get("label"), "city": s.get("city"),
                                            "online": s.get("online"), "listeners": s.get("listeners"),
                                            "external": s.get("external")}
                                           for s in (st.get("stations") or [])]}
                except ValueError:
                    pass
            break
    pub, _ = _rc25_http(RC25_PUBLIC + "/healthz", timeout=6)
    with _rc25_lock:
        _rc25.update(ts=time.time(), nodes=[n for n in out if n], aire=aire, publico=pub)


def rc25_loop():
    while True:
        try:
            rc25_collect()
        except Exception as e:
            log("radioclub25 status error: %s" % e)
        time.sleep(30)


def _rc25_snapshot():
    with _rc25_lock:
        return {"ts": _rc25["ts"], "nodes": list(_rc25["nodes"]), "aire": _rc25["aire"],
                "publico": _rc25.get("publico"), "url": RC25_PUBLIC}


def start_rc25_op(accion, nodo=""):
    """accion: check | sync | deploy | promover (nodo B/C). Una operación a la vez."""
    a = RC25_REPO["10.10.0.2"]
    if accion == "check":
        ip = "10.10.0.2"
        script = ("cd %s && echo '== Código (copia de trabajo de A)' && git rev-parse --abbrev-ref HEAD && git log --oneline -1 && "
                  "echo \"cambios sin commit: $(git status --short | wc -l)\" && "
                  "echo '== Contenedores' && docker compose ps --format '{{.Service}} {{.Status}}' && "
                  "echo '== Edge' && curl -s -m 4 -o /dev/null -w 'healthz %%{http_code}\\n' http://10.10.0.2:8125/healthz && "
                  "echo '== Última sincronización' && tail -n 3 /home/lima/logs/radioclub25-sync.log 2>/dev/null; "
                  "echo '== Disco' && df -h / | tail -1 && "
                  "for f in scripts/ha/*.sh; do bash -n \"$f\" || echo \"ERROR sintaxis $f\"; done && echo 'scripts OK'") % a
    elif accion == "sync":
        ip, script = "10.10.0.2", "cd %s && scripts/ha/sincronizar-nodos.sh 2>&1" % a
    elif accion == "deploy":
        ip = "10.10.0.2"
        script = ("cd %s && docker compose up -d --build 2>&1 | grep -v -E '^ *(#|=>)' ; "
                  "sleep 4 && curl -s -m 4 -o /dev/null -w 'edge A healthz %%{http_code}\\n' http://10.10.0.2:8125/healthz && "
                  "scripts/ha/desplegar-nodos.sh 2>&1") % a
    elif accion == "promover":
        ip = LETTER_IP.get(nodo)
        if ip not in ("10.10.0.3", "10.10.0.4"):
            return {"ok": False, "error": "solo se promueve B o C"}
        script = "cd %s && CONFIRMAR=si scripts/ha/promover.sh 2>&1" % RC25_REPO[ip]
    else:
        return {"ok": False, "error": "acción inválida"}
    etiqueta = {"check": "verificar", "sync": "sincronizar B y C", "deploy": "redesplegar (A + B/C)",
                "promover": "promover Nodo %s" % nodo}[accion]
    with _rc25_op_lock:
        if _rc25_op["running"]:
            return {"ok": False, "error": "ya hay una operación de RadioClub25 en curso"}
        _rc25_op.update(running=True, op=etiqueta, started=time.time(), output="", rc=None)
    log("RADIOCLUB25 %s desde panel -> %s" % (etiqueta, ip))

    def run():
        info = NODES[ip]
        cmd = ["ssh", "-i", SSH_KEY, "-p", str(info["port"]), "-o", "BatchMode=yes",
               "-o", "StrictHostKeyChecking=no", "-o", "ConnectTimeout=10",
               "%s@%s" % (info["user"], ip), script]
        try:
            p = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
            for line in p.stdout:
                with _rc25_op_lock:
                    _rc25_op["output"] += line
            p.wait(timeout=1800)
            rc = p.returncode
        except Exception as e:
            with _rc25_op_lock:
                _rc25_op["output"] += "\nERROR: %s" % e
            rc = -1
        with _rc25_op_lock:
            _rc25_op["running"] = False
            _rc25_op["rc"] = rc
        log("RADIOCLUB25 %s terminado rc=%s" % (etiqueta, rc))
        threading.Thread(target=rc25_collect, daemon=True).start()

    threading.Thread(target=run, daemon=True).start()
    return {"ok": True, "op": etiqueta}


# ── HTTP ──────────────────────────────────────────────────────────────────────

class Handler(BaseHTTPRequestHandler):
    server_version = "HAPanel/1.0"

    def _send(self, code, body, ctype="application/json"):
        data = body if isinstance(body, bytes) else json.dumps(body).encode()
        self.send_response(code)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(data)))
        self.send_header("Cache-Control", "no-store")
        self.end_headers()
        self.wfile.write(data)

    def log_message(self, fmt, *args):
        pass  # silenciar access log (HAProxy ya loguea)

    def _path(self):
        p = self.path.split("?")[0]
        if p.startswith(PREFIX):
            p = p[len(PREFIX):] or "/"
        return p

    def _query(self):
        if "?" not in self.path:
            return {}
        out = {}
        for kv in self.path.split("?", 1)[1].split("&"):
            if "=" in kv:
                k, v = kv.split("=", 1)
                out[k] = v
        return out

    def do_GET(self):
        p = self._path()
        if p in ("/", "/index.html"):
            try:
                with open(FRONTEND, "rb") as f:
                    return self._send(200, f.read(), "text/html; charset=utf-8")
            except OSError:
                return self._send(500, {"error": "frontend no encontrado"})
        if p == "/api/overview":
            return self._send(200, {
                "ts": time.time(),
                "topology": topology(),
                "problems": problems(),
                "haproxy": haproxy_stats(),
                "services": services_status(),
                "wireguard": wireguard_peers(),
                "sys": sys_resources(),
                "current_master": current_master(),
                "failover": {k: _failover[k] for k in ("running", "node", "rc")},
                "deploy": {k: _deploy[k] for k in ("running", "target", "rc")},
                "nodes_sys": _node_res_snapshot(),
                "semisync": _semisync_snapshot(),
                "decoupled": decoupled_letters(),
                "nodeop": {k: _nodeop[k] for k in ("running", "op", "node", "rc")},
                "kipu": _kipu_snapshot(),
                "kipu_deploy": {k: _kipu_deploy[k] for k in ("running", "target", "rc")},
                "kipu_op": {k: _kipu_op[k] for k in ("running", "op", "rc")},
                "radioclub25": _rc25_snapshot(),
                "rc25_op": {k: _rc25_op[k] for k in ("running", "op", "rc")},
            })
        if p == "/api/logs":
            q = self._query()
            name = q.get("name", "events")
            files = {"events": EVENTS_LOG, "orchestrator": "/var/log/orchestrator/orchestrator.log",
                     "bot": "/var/log/orchestrator/bot.log", "panel": PANEL_LOG,
                     "kipu-ha": "/var/log/kipu-ha/eventos.log"}
            if name not in files:
                return self._send(400, {"error": "log desconocido"})
            n = min(int(q.get("lines", "80") or 80), 500)
            return self._send(200, {"name": name, "content": tail_file(files[name], n)})
        if p == "/api/failover/status":
            with _failover_lock:
                st = dict(_failover)
            return self._send(200, st)
        if p == "/api/deploy/status":
            with _deploy_lock:
                st = dict(_deploy)
            return self._send(200, st)
        if p == "/api/postcheck/status":
            with _postcheck_lock:
                st = dict(_postcheck)
            return self._send(200, st)
        if p == "/api/kipu/ha/status":
            with _kipu_op_lock:
                st = dict(_kipu_op)
            return self._send(200, st)
        if p == "/api/radioclub25/status":
            with _rc25_op_lock:
                st = dict(_rc25_op)
            return self._send(200, st)
        if p == "/api/kipu/deploy/status":
            with _kipu_deploy_lock:
                st = dict(_kipu_deploy)
            return self._send(200, st)
        if p == "/api/node/status":
            with _nodeop_lock:
                st = dict(_nodeop)
            return self._send(200, st)
        return self._send(404, {"error": "no encontrado"})

    def do_POST(self):
        # Anti-CSRF: exigir header custom (fuerza preflight CORS en cross-origin)
        if self.headers.get("X-Panel") != "1":
            return self._send(403, {"error": "falta header X-Panel"})
        p = self._path()
        try:
            length = int(self.headers.get("Content-Length", "0"))
            body = json.loads(self.rfile.read(length) or b"{}")
        except (ValueError, json.JSONDecodeError):
            return self._send(400, {"error": "JSON inválido"})
        if p == "/api/haproxy/server":
            res = haproxy_server_action(body.get("backend", ""), body.get("server", ""),
                                        body.get("action", ""))
            return self._send(200 if res.get("ok") else 400, res)
        if p == "/api/failover":
            if body.get("confirm") != "SI":
                return self._send(400, {"ok": False, "error": "confirmación requerida (SI)"})
            res = start_failover(body.get("node", ""))
            return self._send(200 if res.get("ok") else 400, res)
        if p == "/api/deploy":
            check = bool(body.get("check"))
            if not check and body.get("confirm") != "DEPLOY":
                return self._send(400, {"ok": False, "error": "confirmación requerida (DEPLOY)"})
            res = start_deploy(check=check)
            return self._send(200 if res.get("ok") else 400, res)
        if p == "/api/postcheck":
            res = start_postcheck()
            return self._send(200 if res.get("ok") else 400, res)
        if p == "/api/kipu/ha":
            accion, nodo = body.get("accion", ""), body.get("nodo", "")
            requerido = {"failover": "PROMOVER", "reintegrar": "REINTEGRAR"}.get(accion)
            if requerido and body.get("confirm") != requerido:
                return self._send(400, {"ok": False, "error": "confirmación requerida (%s)" % requerido})
            res = start_kipu_op(accion, nodo)
            return self._send(200 if res.get("ok") else 400, res)
        if p == "/api/kipu/deploy":
            check = bool(body.get("check"))
            if not check and body.get("confirm") != "KIPU":
                return self._send(400, {"ok": False, "error": "confirmación requerida (KIPU)"})
            res = start_kipu_deploy(check=check)
            return self._send(200 if res.get("ok") else 400, res)
        if p == "/api/radioclub25":
            accion = body.get("accion", "")
            requerido = {"deploy": "RADIO", "promover": "PROMOVER"}.get(accion)
            if requerido and body.get("confirm") != requerido:
                return self._send(400, {"ok": False, "error": "confirmación requerida (%s)" % requerido})
            res = start_rc25_op(accion, body.get("nodo", ""))
            return self._send(200 if res.get("ok") else 400, res)
        if p == "/api/node/decouple":
            if body.get("confirm") != "AISLAR":
                return self._send(400, {"ok": False, "error": "confirmación requerida (AISLAR)"})
            res = start_nodeop("decouple", body.get("node", ""))
            return self._send(200 if res.get("ok") else 400, res)
        if p == "/api/node/couple":
            if body.get("confirm") != "CLONAR":
                return self._send(400, {"ok": False, "error": "confirmación requerida (CLONAR)"})
            res = start_nodeop("couple", body.get("node", ""))
            return self._send(200 if res.get("ok") else 400, res)
        return self._send(404, {"error": "no encontrado"})


def main():
    threading.Thread(target=node_resources_loop, daemon=True).start()
    threading.Thread(target=kipu_loop, daemon=True).start()
    threading.Thread(target=rc25_loop, daemon=True).start()
    srv = ThreadingHTTPServer(BIND, Handler)
    log("ha-panel iniciado en %s:%s" % BIND)
    srv.serve_forever()


if __name__ == "__main__":
    main()
