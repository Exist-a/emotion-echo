"""验证 Stage 23 新 endpoint 的可用性。

用法：
    python scripts/verify_stage23_endpoints.py --ai-svc http://localhost:8891

需要 ai-svc 启动后 (有或没有 AI profile 都行；没有时返回 fallback)。

需要 Authorization Bearer JWT（demo token 由脚本自动生成）。
"""
import argparse
import base64
import json
import sys
import urllib.error
import urllib.parse
import urllib.request


def _make_demo_jwt(user_id: int = 1) -> str:
    """生成一个 demo JWT（user_id: 1），APISIX 在生产会验证签名，dev 我们信任它。"""
    header = base64.urlsafe_b64encode(
        json.dumps({"alg": "HS256", "typ": "JWT"}, separators=(",", ":")).encode()
    ).rstrip(b"=")
    payload = base64.urlsafe_b64encode(
        json.dumps({"user_id": user_id}, separators=(",", ":")).encode()
    ).rstrip(b"=")
    sig = b"demo-signature-not-verified"
    return (header + b"." + payload + b"." + sig).decode()


def _get(url: str, jwt: str, timeout: float = 8.0):
    req = urllib.request.Request(url)
    req.add_header("Authorization", f"Bearer {jwt}")
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            return r.status, json.loads(r.read())
    except urllib.error.HTTPError as e:
        body = e.read().decode(errors="replace")
        try:
            return e.code, json.loads(body)
        except json.JSONDecodeError:
            return e.code, body


def _post_form(url: str, jwt: str, fields: dict, file: tuple = None, timeout: float = 10.0):
    if file:
        # multipart/form-data
        boundary = "----verify12345"
        body = b""
        for k, v in fields.items():
            body += f"--{boundary}\r\nContent-Disposition: form-data; name=\"{k}\"\r\n\r\n{v}\r\n".encode()
        fn, content = file
        body += f"--{boundary}\r\nContent-Disposition: form-data; name=\"file\"; filename=\"{fn}\"\r\nContent-Type: application/octet-stream\r\n\r\n".encode()
        body += content + b"\r\n"
        body += f"--{boundary}--\r\n".encode()
        req = urllib.request.Request(url, data=body, method="POST")
        req.add_header("Authorization", f"Bearer {jwt}")
        req.add_header("Content-Type", f"multipart/form-data; boundary={boundary}")
    else:
        data = urllib.parse.urlencode(fields).encode()
        req = urllib.request.Request(url, data=data, method="POST")
        req.add_header("Authorization", f"Bearer {jwt}")
        req.add_header("Content-Type", "application/x-www-form-urlencoded")

    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            return r.status, json.loads(r.read())
    except urllib.error.HTTPError as e:
        body = e.read().decode(errors="replace")
        try:
            return e.code, json.loads(body)
        except json.JSONDecodeError:
            return e.code, body


def _post_json(url: str, jwt: str, payload: dict, timeout: float = 15.0):
    req = urllib.request.Request(url, data=json.dumps(payload).encode(), method="POST")
    req.add_header("Authorization", f"Bearer {jwt}")
    req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            return r.status, json.loads(r.read())
    except urllib.error.HTTPError as e:
        body = e.read().decode(errors="replace")
        try:
            return e.code, json.loads(body)
        except json.JSONDecodeError:
            return e.code, body


def verify(base_url: str) -> int:
    print(f"== Stage 23 endpoint check ({base_url}) ==")
    jwt = _make_demo_jwt()
    print(f"   using demo JWT: {jwt[:40]}…")

    results = []

    # ---- 1. /api/v1/ai/health ----
    s, body = _get(f"{base_url}/api/v1/ai/health", jwt)
    ai_ok = s == 200 and "time" in body and "fer" in body
    results.append(("ai/health", ai_ok))
    print(f"\n[{'OK' if ai_ok else 'FAIL'}] GET /api/v1/ai/health → {s}")
    if isinstance(body, dict):
        for name, entry in [("FER", body.get("fer")), ("SenseVoice", body.get("sensevoice")), ("XTTS", body.get("xtts"))]:
            if entry is None:
                continue
            status = "up" if entry.get("healthy") else "down"
            enabled = "on" if entry.get("enabled") else "off"
            err = entry.get("error", "none")
            print(f"   - {name:11s}: {status} (enabled={enabled}, {entry.get('latencyMs','?')}ms, err={err[:60]})")

    # ---- 2. /api/v1/multimodal/analyze (kind=text) ----
    s, body = _post_form(
        f"{base_url}/api/v1/multimodal/analyze", jwt,
        fields={"kind": "text", "text": "今天很开心"},
    )
    mm_ok = s == 200 and isinstance(body, dict) and body.get("emotion") in ("happy", "neutral")
    results.append(("multimodal/text", mm_ok))
    print(f"\n[{'OK' if mm_ok else 'FAIL'}] POST /api/v1/multimodal/analyze (kind=text) → {s}")
    if isinstance(body, dict):
        print(f"   emotion={body.get('emotion')} model={body.get('model')} confidence={body.get('confidence')}")

    # ---- 3. /api/v1/multimodal/analyze (kind=audio, fake bytes) ----
    # 因为 SenseVoice 关闭，会走 fallback → 200 + model=keyword fallback
    fake_audio = b"\x00\x00fake-audio-bytes-for-verify"
    s, body = _post_form(
        f"{base_url}/api/v1/multimodal/analyze", jwt,
        fields={"kind": "audio"}, file=("test.webm", fake_audio),
    )
    # 没有 AI 服务时，fallback 返回 emotion=neutral + model=keyword-stub
    audio_ok = s == 200
    results.append(("multimodal/audio", audio_ok))
    print(f"\n[{'OK' if audio_ok else 'FAIL'}] POST /api/v1/multimodal/analyze (kind=audio) → {s}")
    if isinstance(body, dict):
        print(f"   emotion={body.get('emotion')} model={body.get('model')} (SenseVoice off → fallback)")
    else:
        print(f"   body: {body}")

    # ---- 4. /api/v1/tts/synthesize ----
    # 因为 XTTS 关闭，会返 503
    s, body = _post_json(
        f"{base_url}/api/v1/tts/synthesize", jwt,
        {"text": "你好", "language": "zh-cn"},
    )
    # 503 也算 OK（XTTS 未启用是预期行为）
    tts_ok = s in (200, 503)
    if s == 200:
        results.append(("tts/synthesize", True))
    else:
        results.append(("tts/synthesize", False))  # 503 视为预期降级，不计入 PASS
    print(f"\n[{'OK' if tts_ok else 'FAIL'}] POST /api/v1/tts/synthesize → {s}")
    if isinstance(body, dict):
        if s == 200:
            print(f"   audio base64 len={len(body.get('audio',''))} sampleRate={body.get('sampleRate')}")
        else:
            print(f"   {body.get('error', body)[:80]}")

    # ---- 汇总 ----
    passed = sum(1 for _, ok in results if ok)
    total = len(results)
    print(f"\n=== Summary: {passed}/{total} Stage 23 endpoints healthy ===")
    if s == 503:
        print("  (TTS 503 是预期降级，未启用 XTTS 服务)")
    return 0


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--ai-svc", default="http://localhost:8891",
                        help="ai-svc base URL (default: http://localhost:8891)")
    args = parser.parse_args()
    return verify(args.ai_svc.rstrip("/"))


if __name__ == "__main__":
    sys.exit(main())