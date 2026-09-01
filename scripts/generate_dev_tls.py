#!/usr/bin/env python3
"""
Stage 36-B3 增补 / Stage 18 历史补回: 生成 dev TLS 证书 (Python cryptography)

为 emotion-llm-service (gRPC server) + emotion-echo-ai-svc (gRPC client)
生成 mTLS 自签名证书。dev 环境一次性产出，**生产应替换为 cert-manager + Vault**
（见 docs/stage-18-grpc-mtls.md §8 TODO）。

Usage:
    python scripts/generate_dev_tls.py [validity_days]

Output (writes to deploy/tls/):
    ca.crt / ca.key               - 自签 CA（10y）
    llm-server.crt / llm-server.key - server cert (CN=emotion-llm-service, SAN=DNS:localhost,...)
    ai-client.crt  / ai-client.key  - client cert (CN=emotion-echo-ai-svc)

Note: .key files 是私钥，gitignored；.crt 文件可入仓（公钥证书）。
"""

import sys
from datetime import datetime, timedelta, timezone
from ipaddress import IPv4Address
from pathlib import Path

from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa
from cryptography.x509.oid import NameOID

REPO_ROOT = Path(__file__).resolve().parent.parent
TLS_DIR = REPO_ROOT / "deploy" / "tls"
CA_VALIDITY_DAYS = 3650  # 10y


def make_key() -> rsa.RSAPrivateKey:
    """生成 2048-bit RSA 私钥"""
    return rsa.generate_private_key(public_exponent=65537, key_size=2048)


def make_name(cn: str) -> x509.Name:
    """构造证书 subject，统一格式"""
    return x509.Name([
        x509.NameAttribute(NameOID.COUNTRY_NAME, "CN"),
        x509.NameAttribute(NameOID.STATE_OR_PROVINCE_NAME, "Dev"),
        x509.NameAttribute(NameOID.ORGANIZATION_NAME, "Emotion-Echo"),
        x509.NameAttribute(NameOID.ORGANIZATIONAL_UNIT_NAME, "Dev"),
        x509.NameAttribute(NameOID.COMMON_NAME, cn),
    ])


def write_key(key: rsa.RSAPrivateKey, path: Path) -> None:
    path.write_bytes(
        key.private_bytes(
            encoding=serialization.Encoding.PEM,
            format=serialization.PrivateFormat.PKCS8,
            encryption_algorithm=serialization.NoEncryption(),
        )
    )
    path.chmod(0o600)


def write_cert(cert: x509.Certificate, path: Path) -> None:
    path.write_bytes(cert.public_bytes(serialization.Encoding.PEM))


def make_self_signed_ca(key: rsa.RSAPrivateKey, cn: str, days: int) -> x509.Certificate:
    subject = issuer = make_name(cn)
    now = datetime.now(timezone.utc)
    return (
        x509.CertificateBuilder()
        .subject_name(subject)
        .issuer_name(issuer)
        .public_key(key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(now)
        .not_valid_after(now + timedelta(days=days))
        .add_extension(x509.BasicConstraints(ca=True, path_length=None), critical=True)
        .sign(key, hashes.SHA256())
    )


def make_leaf_cert(
    leaf_key: rsa.RSAPrivateKey,
    ca_key: rsa.RSAPrivateKey,
    ca_cert: x509.Certificate,
    cn: str,
    san_dns: list[str],
    san_ip: list[str],
    days: int,
) -> x509.Certificate:
    # san_dns / san_ip 是裸 hostname / IP（如 ["localhost", "emotion-llm-service"]），
    # 包装成 cryptography 的 DNSName / IPAddress 对象。
    san_entries = [x509.DNSName(name) for name in san_dns] + [x509.IPAddress(IPv4Address(ip)) for ip in san_ip]
    now = datetime.now(timezone.utc)
    builder = (
        x509.CertificateBuilder()
        .subject_name(make_name(cn))
        .issuer_name(ca_cert.subject)
        .public_key(leaf_key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(now)
        .not_valid_after(now + timedelta(days=days))
        .add_extension(
            x509.SubjectAlternativeName(san_entries),
            critical=False,
        )
    )
    return builder.sign(ca_key, hashes.SHA256())


def main() -> int:
    validity_days = int(sys.argv[1]) if len(sys.argv) > 1 else 365

    TLS_DIR.mkdir(parents=True, exist_ok=True)

    # Step 1: CA
    print(f"[tls] generating CA ({CA_VALIDITY_DAYS // 365}y)...")
    ca_key = make_key()
    ca_cert = make_self_signed_ca(ca_key, "emotion-echo-dev-ca", CA_VALIDITY_DAYS)
    write_key(ca_key, TLS_DIR / "ca.key")
    write_cert(ca_cert, TLS_DIR / "ca.crt")

    # Step 2: server cert (emotion-llm-service)
    print(f"[tls] generating llm-server ({validity_days}d)...")
    server_key = make_key()
    server_cert = make_leaf_cert(
        server_key, ca_key, ca_cert,
        cn="emotion-llm-service",
        san_dns=["localhost", "emotion-llm-service"],
        san_ip=["127.0.0.1"],
        days=validity_days,
    )
    write_key(server_key, TLS_DIR / "llm-server.key")
    write_cert(server_cert, TLS_DIR / "llm-server.crt")

    # Step 3: client cert (emotion-echo-ai-svc)
    print(f"[tls] generating ai-client ({validity_days}d)...")
    client_key = make_key()
    client_cert = make_leaf_cert(
        client_key, ca_key, ca_cert,
        cn="emotion-echo-ai-svc",
        san_dns=["emotion-echo-ai-svc"],
        san_ip=[],
        days=validity_days,
    )
    write_key(client_key, TLS_DIR / "ai-client.key")
    write_cert(client_cert, TLS_DIR / "ai-client.crt")

    # Summary
    print()
    print(f"[tls] ✅ generated {len(list(TLS_DIR.glob('*.crt'))) + len(list(TLS_DIR.glob('*.key')))} files in {TLS_DIR}:")
    for f in sorted(TLS_DIR.glob("*")):
        if f.is_file():
            print(f"     {f.name:25s} {f.stat().st_size:5d} bytes")

    print()
    print("[tls] llm-server.crt SAN:")
    san_ext = server_cert.extensions.get_extension_for_class(x509.SubjectAlternativeName)
    print(f"     {san_ext.value}")

    print()
    print("[tls] 下一步: 重启 emotion-llm-service 加载新证书")
    print("       docker compose restart emotion-llm-service")
    return 0


if __name__ == "__main__":
    sys.exit(main())