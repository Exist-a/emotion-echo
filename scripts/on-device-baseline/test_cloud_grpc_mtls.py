"""cloud_grpc mTLS 烟测（Lane O · 端侧化阶段一 · OND-F-09 修复轮）。

背景（`docs/plans/on-device-t3-lessons-learned-2026-09-28.md` §一）：
T3 实测发现 baseline `cloud_grpc.py` 用 `grpc.insecure_channel`，而
emotion-llm-service v0.1.2 默认 `TLS_ENABLED=1` + `TLS_REQUIRE_CLIENT_AUTH=1`
（`emotion-llm-service/grpc_server.py:461-466`）→ 握手失败 →
`grpc_unreachable:FutureTimeoutError:mock_fallback` → N=13 全 mock，
D-26.2 ADR §五"真机基线"验收契约从未成立。

本文件是**回归钉**：起一个真实 mTLS gRPC server（`require_client_auth=True`），
断言 baseline 客户端带证书能连上并拿到真实回复。旧实现（insecure_channel）
在本文件下**必然 RED**——这是本测试存在的理由。

证书自签自生成（tmp_path），**不依赖 deploy/tls/**：
`.gitignore:126 *.crt` 让 dev 证书不入仓，fresh clone 上 deploy/tls 是空的；
测试必须自足。签发形状对齐 `scripts/generate_dev_tls.py`（同 CA / 同 SAN 口径）。

跑法（仓库根）：
    python -m pytest scripts/on-device-baseline/test_cloud_grpc_mtls.py -v
"""
from __future__ import annotations

import sys
from concurrent import futures
from datetime import datetime, timedelta, timezone
from pathlib import Path
from types import SimpleNamespace

import grpc
import pytest

HERE = Path(__file__).resolve().parent
if str(HERE) not in sys.path:
    sys.path.insert(0, str(HERE))

from model_fn_factory import make_model_fn  # noqa: E402
from model_fns.cloud_grpc import (  # noqa: E402
    DEFAULT_CA_CERT,
    DEFAULT_CLIENT_CERT,
    DEFAULT_CLIENT_KEY,
    REPO_ROOT,
    TlsConfigError,
    _resolve_tls_config,
    build_channel,
)

# pb2 与 server 配套，需 emotion-llm-service/ 在 sys.path（与 cloud_grpc._import_pb 同款妥协）
REPO_ROOT = HERE.parents[1]
LLM_SVC_DIR = REPO_ROOT / "emotion-llm-service"
if str(LLM_SVC_DIR) not in sys.path:
    sys.path.insert(0, str(LLM_SVC_DIR))

import emotion_llm_pb2 as pb2  # noqa: E402
import emotion_llm_pb2_grpc as pb2_grpc  # noqa: E402


# ============================================================
# 测试夹具：一次性签发 CA / server / client 证书
# ============================================================

def _make_name(cn: str):
    from cryptography import x509
    from cryptography.x509.oid import NameOID

    return x509.Name([
        x509.NameAttribute(NameOID.COUNTRY_NAME, "CN"),
        x509.NameAttribute(NameOID.COMMON_NAME, cn),
    ])


def _write_pem(path: Path, obj, *, private: bool) -> None:
    from cryptography.hazmat.primitives import serialization

    if private:
        path.write_bytes(obj.private_bytes(
            encoding=serialization.Encoding.PEM,
            format=serialization.PrivateFormat.PKCS8,
            encryption_algorithm=serialization.NoEncryption(),
        ))
    else:
        path.write_bytes(obj.public_bytes(serialization.Encoding.PEM))


@pytest.fixture(scope="module")
def certs(tmp_path_factory):
    """签发一套与 generate_dev_tls.py 同形状的 dev 证书（SAN 含 localhost）。"""
    from cryptography import x509
    from cryptography.hazmat.primitives import hashes
    from cryptography.hazmat.primitives.asymmetric import rsa

    out = tmp_path_factory.mktemp("tls")
    now = datetime.now(timezone.utc)

    ca_key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    ca_name = _make_name("emotion-echo-dev-ca")
    ca_cert = (
        x509.CertificateBuilder()
        .subject_name(ca_name).issuer_name(ca_name)
        .public_key(ca_key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(now).not_valid_after(now + timedelta(days=1))
        .add_extension(x509.BasicConstraints(ca=True, path_length=None), critical=True)
        .sign(ca_key, hashes.SHA256())
    )
    _write_pem(out / "ca.crt", ca_cert, private=False)
    _write_pem(out / "ca.key", ca_key, private=True)

    def leaf(cn: str, san_dns: list[str], san_ip: list[str], filename: str):
        from ipaddress import IPv4Address

        key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
        san = [x509.DNSName(n) for n in san_dns] + [
            x509.IPAddress(IPv4Address(ip)) for ip in san_ip
        ]
        builder = (
            x509.CertificateBuilder()
            .subject_name(_make_name(cn)).issuer_name(ca_cert.subject)
            .public_key(key.public_key())
            .serial_number(x509.random_serial_number())
            .not_valid_before(now).not_valid_after(now + timedelta(days=1))
            .add_extension(x509.SubjectAlternativeName(san), critical=False)
            .add_extension(x509.ExtendedKeyUsage([
                x509.oid.ExtendedKeyUsageOID.SERVER_AUTH,
                x509.oid.ExtendedKeyUsageOID.CLIENT_AUTH,
            ]), critical=False)
        )
        cert = builder.sign(ca_key, hashes.SHA256())
        _write_pem(out / f"{filename}.crt", cert, private=False)
        _write_pem(out / f"{filename}.key", key, private=True)

    # SAN 口径对齐 scripts/generate_dev_tls.py：server 兼带 DNS + IP 127.0.0.1。
    # 少签 IP 会让连 127.0.0.1:port 的客户端证书校验失败（实测踩过）。
    leaf("emotion-llm-service", ["localhost", "emotion-llm-service"], ["127.0.0.1"], "server")
    leaf("emotion-echo-baseline", ["localhost"], ["127.0.0.1"], "client")

    return SimpleNamespace(
        ca_crt=out / "ca.crt",
        server_crt=out / "server.crt",
        server_key=out / "server.key",
        client_crt=out / "client.crt",
        client_key=out / "client.key",
    )


def _tls_env(certs, **overrides) -> dict[str, str]:
    env = {
        "TLS_ENABLED": "1",
        "TLS_CA_CERT": str(certs.ca_crt),
        "TLS_CLIENT_CERT": str(certs.client_crt),
        "TLS_CLIENT_KEY": str(certs.client_key),
    }
    env.update(overrides)
    return env


@pytest.fixture
def mtls_server(certs):
    """真实 mTLS gRPC server：require_client_auth=True（与 llm-service 同配置）。"""
    server_creds = grpc.ssl_server_credentials(
        [(certs.server_key.read_bytes(), certs.server_crt.read_bytes())],
        root_certificates=certs.ca_crt.read_bytes(),
        require_client_auth=True,
    )
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=2))
    pb2_grpc.add_EmotionLLMServiceServicer_to_server(_EchoServicer(), server)
    port = server.add_secure_port("127.0.0.1:0", server_creds)
    server.start()
    try:
        yield SimpleNamespace(target=f"127.0.0.1:{port}", port=port)
    finally:
        server.stop(None)


class _EchoServicer(pb2_grpc.EmotionLLMServiceServicer):
    """回 3 帧，形状对齐真 llm-service 的流式 ChatChunk。"""

    def ChatCompletion(self, request, context):
        assert context is not None, "服务端应能拿到带证书的 context（证明 mTLS 生效）"
        yield pb2.ChatChunk(delta_content="我在", done=False)
        yield pb2.ChatChunk(delta_content="听你说", done=False)
        yield pb2.ChatChunk(delta_content="。", done=True, model="test-qwen3")


# ============================================================
# 核心回归钉：带证书能连上（旧 insecure_channel 必然 RED）
# ============================================================

def test_secure_channel_handshake_succeeds_against_mtls_server(certs, mtls_server):
    cfg = _resolve_tls_config(env=_tls_env(certs))
    assert cfg.enabled is True
    channel = build_channel(mtls_server.target, cfg)
    try:
        grpc.channel_ready_future(channel).result(timeout=5.0)
    finally:
        channel.close()


def test_model_fn_reaches_mtls_server_and_gets_real_reply(certs, mtls_server):
    """端到端：走 model_fn 拿到服务端真回复，**不是** mock fallback。

    这是 OND-F-09 的直接回归证明：旧实现在此返回
    fallback_reason=grpc_unreachable:...:mock_fallback。
    """
    fn = make_model_fn(
        "cloud_grpc",
        target=mtls_server.target,
        tls_env=_tls_env(certs),
        timeout_s=5.0,
    )
    r = fn({"id": "t-mtls", "layer": "daily", "input": "今天好累", "expect": {}})
    assert r.fallback_reason == ""
    assert r.text == "我在听你说。"
    assert r.model == "test-qwen3"


def test_server_really_requires_client_cert(certs, mtls_server):
    """反向钉：只给 CA 不给客户端证书 → 握手失败。

    若此用例不成立（无证书也能连），说明服务端配置没起到"要求 mTLS"的作用，
    上面的成功用例就不构成有效证据。

    这里直接用 grpc 原生 API 构造"仅服务端认证"的通道，不走 _resolve_tls_config
    —— 后者在缺客户端证书时会 fail-loud 抛错（那是配置层契约，与握手行为两回事）。
    """
    creds = grpc.ssl_channel_credentials(
        root_certificates=certs.ca_crt.read_bytes(),
    )
    channel = grpc.secure_channel(mtls_server.target, creds)
    try:
        with pytest.raises(Exception):
            grpc.channel_ready_future(channel).result(timeout=3.0)
    finally:
        channel.close()


def test_insecure_channel_cannot_reach_mtls_server(mtls_server):
    """OND-F-09 根因的characterization：insecure_channel 对上 mTLS server 必握手失败。

    本用例在修复前后都通过——它记录的是"旧写法为什么错"，防止有人改回 insecure。
    """
    channel = grpc.insecure_channel(mtls_server.target)
    try:
        with pytest.raises(Exception):
            grpc.channel_ready_future(channel).result(timeout=3.0)
    finally:
        channel.close()


# ============================================================
# TLS 配置解析：显式开启 / 显式关闭 / 自动 / 缺件 fail-loud
# ============================================================

def test_tls_env_paths_override_repo_defaults(certs):
    """env 给了路径就用 env 的，不回落 deploy/tls（显式配置优先）。"""
    cfg = _resolve_tls_config(env=_tls_env(certs))
    assert cfg.ca_cert == certs.ca_crt
    assert cfg.client_cert == certs.client_crt
    assert cfg.client_key == certs.client_key


def test_tls_enabled_with_missing_cert_raises_instead_of_downgrading(certs, tmp_path):
    """fail-loud（AGENTS §3.2 不静默降级）：要 TLS 却给不出证书 → 抛错，
    绝不悄悄退回 insecure（那正是 OND-F-09 的病根）。"""
    with pytest.raises(TlsConfigError, match="TLS_CLIENT_KEY"):
        _resolve_tls_config(env=_tls_env(
            certs, TLS_CLIENT_KEY=str(tmp_path / "nope.key"),
        ))


def test_tls_disabled_explicitly_yields_insecure(certs):
    cfg = _resolve_tls_config(env={"TLS_ENABLED": "0"})
    assert cfg.enabled is False


def test_tls_auto_enables_when_cert_files_all_present(certs):
    """TLS_ENABLED 未设 + 三证书齐全 → 自动走 mTLS（dev compose 的真实形态）。"""
    cfg = _resolve_tls_config(env={
        "TLS_CA_CERT": str(certs.ca_crt),
        "TLS_CLIENT_CERT": str(certs.client_crt),
        "TLS_CLIENT_KEY": str(certs.client_key),
    })
    assert cfg.enabled is True


def test_tls_auto_stays_insecure_when_no_cert_files(tmp_path):
    """TLS_ENABLED 未设 + 一个证书都没有 → 明文（fresh clone / 无 TLS 部署）。"""
    cfg = _resolve_tls_config(env={
        "TLS_CA_CERT": str(tmp_path / "missing.crt"),
        "TLS_CLIENT_CERT": str(tmp_path / "missing.crt"),
        "TLS_CLIENT_KEY": str(tmp_path / "missing.key"),
    })
    assert cfg.enabled is False


def test_tls_auto_raises_on_partial_cert_set(certs, tmp_path):
    """部分证书存在 = 配错了，不许当成"没配"而退回明文。"""
    with pytest.raises(TlsConfigError, match="部分"):
        _resolve_tls_config(env={
            "TLS_CA_CERT": str(certs.ca_crt),
            "TLS_CLIENT_CERT": "",
            "TLS_CLIENT_KEY": "",
        })


def test_tls_paths_default_to_repo_deploy_tls(certs, monkeypatch):
    """未设 env 时的缺省路径 = deploy/tls/{ca,ai-client}——与 compose 挂载同名。

    断言模块级缺省常量而非返回的 TlsConfig：仓库里 .crt 不入仓（gitignore:126），
    fresh clone 上 deploy/tls 为空时自动模式会返回 enabled=False（三证书皆 None），
    那是预期行为，不该让本用例随 clone 状态而飘。
    """
    for k in ("TLS_ENABLED", "TLS_CA_CERT", "TLS_CLIENT_CERT", "TLS_CLIENT_KEY"):
        monkeypatch.delenv(k, raising=False)
    assert DEFAULT_CA_CERT.name == "ca.crt"
    assert DEFAULT_CLIENT_CERT.name == "ai-client.crt"
    assert DEFAULT_CLIENT_KEY.name == "ai-client.key"
    for path in (DEFAULT_CA_CERT, DEFAULT_CLIENT_CERT, DEFAULT_CLIENT_KEY):
        assert str(path).replace("\\", "/").startswith(str(REPO_ROOT).replace("\\", "/"))
        assert str(path).replace("\\", "/").endswith("deploy/tls/" + path.name)

    # 自动模式 + 证书齐全 → 走 mTLS；一个都没有 → 明文（不抛）
    assert _resolve_tls_config(env=None).enabled == (
        DEFAULT_CA_CERT.exists() and DEFAULT_CLIENT_CERT.exists() and DEFAULT_CLIENT_KEY.exists()
    )


def test_tls_true_aliases_are_accepted(certs):
    for raw in ("1", "true", "TRUE", "yes"):
        cfg = _resolve_tls_config(env=_tls_env(certs, TLS_ENABLED=raw))
        assert cfg.enabled is True, f"TLS_ENABLED={raw!r} 应识别为开启"
