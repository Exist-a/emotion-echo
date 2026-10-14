#!/usr/bin/env python3
"""CL-F-14 运行时验证：查 SkyWalking 的服务列表里有没有某个服务。

用法: python sw_services.py <service-substring>
退出码: 0=在列表里, 1=不在, 2=查询失败
"""
import datetime
import json
import sys
import urllib.request

GQL = "http://127.0.0.1:18080/graphql"


def q(query, variables=None):
    body = json.dumps({"query": query, "variables": variables or {}}).encode()
    req = urllib.request.Request(GQL, data=body,
                                 headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=30) as r:
        return json.load(r)


def main():
    want = sys.argv[1] if len(sys.argv) > 1 else "chat-svc"
    now = datetime.datetime.now()
    start = (now - datetime.timedelta(hours=2)).strftime("%Y-%m-%d %H%M")
    end = (now + datetime.timedelta(minutes=30)).strftime("%Y-%m-%d %H%M")
    try:
        res = q('query ($s: String!, $e: String!) {'
                '  getAllServices(duration: {start: $s, end: $e, step: MINUTE}) { id name }'
                '}', {"s": start, "e": end})
    except Exception as exc:  # noqa: BLE001
        print(f"QUERY_FAILED: {exc}")
        return 2
    svcs = (res.get("data") or {}).get("getAllServices")
    if svcs is None:
        print("QUERY_FAILED: " + json.dumps(res)[:300])
        return 2
    names = sorted(s["name"] for s in svcs)
    hit = [n for n in names if want in n]
    print(f"services({len(names)})={names}")
    if hit:
        print(f"FOUND: {hit}")
        return 0
    print(f"ABSENT: no service matching '{want}'")
    return 1


if __name__ == "__main__":
    sys.exit(main())
