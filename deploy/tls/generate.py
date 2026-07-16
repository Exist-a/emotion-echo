"""生成 mTLS 自签名证书（开发/测试用）

生成：
  - ca.crt, ca.key: CA 根证书
  - llm-server.crt, llm-server.key: emotion-llm-service 服务端证书
  - ai-client.crt, ai-client.key: emotion-echo-ai-svc 客户端证书

用法：
  cd deploy/tls && python generate.py

注意：
  - 证书有效期 1 年
  - 生产环境应该用 Vault / cert-manager 签发正式证书
"""
import datetime
import ipaddress
import os
from cryptography import x509
from cryptography.x509.oid import NameOID
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa

OUT_DIR = os.path.dirname(os.path.abspath(__file__))


def gen_key():
    return rsa.generate_private_key(public_exponent=65537, key_size=2048)


def save_key(key, name):
    path = os.path.join(OUT_DIR, name)
    with open(path, "wb") as f:
        f.write(
            key.private_bytes(
                encoding=serialization.Encoding.PEM,
                format=serialization.PrivateFormat.TraditionalOpenSSL,
                encryption_algorithm=serialization.NoEncryption(),
            )
        )
    os.chmod(path, 0o600)  # private key 仅 owner 可读
    return path


def save_cert(cert, name):
    path = os.path.join(OUT_DIR, name)
    with open(path, "wb") as f:
        f.write(cert.public_bytes(serialization.Encoding.PEM))
    return path


def cert_from_csr(csr, issuer_cert, issuer_key, days=365):
    """用 CA 签发证书"""
    builder = (
        x509.CertificateBuilder()
        .subject_name(csr.subject)
        .issuer_name(issuer_cert.subject)
        .public_key(csr.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(datetime.datetime.utcnow())
        .not_valid_after(datetime.datetime.utcnow() + datetime.timedelta(days=days))
    )
    for ext in csr.extensions:
        builder = builder.add_extension(ext.value, critical=ext.critical)
    return builder.sign(issuer_key, hashes.SHA256())


def make_csr(key, cn, san_dns=None, san_ip=None, is_ca=False):
    """构造 CSR（包含 SAN）"""
    name = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, cn)])
    builder = x509.CertificateSigningRequestBuilder().subject_name(name)
    if san_dns or san_ip:
        san_list = []
        if san_dns:
            san_list.extend(x509.DNSName(d) for d in san_dns)
        if san_ip:
            san_list.extend(x509.IPAddress(ipaddress.IPv4Address(i)) for i in san_ip)
        builder = builder.add_extension(x509.SubjectAlternativeName(san_list), critical=False)
    if is_ca:
        builder = builder.add_extension(
            x509.BasicConstraints(ca=True, path_length=None), critical=True
        )
    return builder.sign(key, hashes.SHA256())


def main():
    print("=== Generating mTLS certificates (dev/test) ===")

    # 1. CA
    ca_key = gen_key()
    ca_name = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, "emotion-echo-dev-ca")])
    ca_cert = (
        x509.CertificateBuilder()
        .subject_name(ca_name)
        .issuer_name(ca_name)
        .public_key(ca_key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(datetime.datetime.utcnow())
        .not_valid_after(datetime.datetime.utcnow() + datetime.timedelta(days=3650))  # 10y
        .add_extension(x509.BasicConstraints(ca=True, path_length=None), critical=True)
        .sign(ca_key, hashes.SHA256())
    )
    save_key(ca_key, "ca.key")
    save_cert(ca_cert, "ca.crt")
    print(f"[CA] ca.crt + ca.key (10 years)")

    # 2. Server cert (emotion-llm-service)
    server_key = gen_key()
    server_csr = make_csr(
        server_key,
        cn="emotion-llm-service",
        san_dns=["localhost", "emotion-llm-service", "emotion-llm"],
        san_ip=["127.0.0.1"],
    )
    server_cert = cert_from_csr(server_csr, ca_cert, ca_key)
    save_key(server_key, "llm-server.key")
    save_cert(server_cert, "llm-server.crt")
    print(f"[Server] llm-server.crt + llm-server.key (1 year, SAN=localhost)")

    # 3. Client cert (emotion-echo-ai-svc)
    client_key = gen_key()
    client_csr = make_csr(client_key, cn="emotion-echo-ai-svc")
    client_cert = cert_from_csr(client_csr, ca_cert, ca_key)
    save_key(client_key, "ai-client.key")
    save_cert(client_cert, "ai-client.crt")
    print(f"[Client] ai-client.crt + ai-client.key (1 year, CN=emotion-echo-ai-svc)")

    print(f"\nFiles written to {OUT_DIR}")
    print("\nTo use:")
    print("  # Server (Python):")
    print("  with open('ca.crt','rb') as f: ca = f.read()")
    print("  with open('llm-server.crt','rb') as f: cert = f.read()")
    print("  with open('llm-server.key','rb') as f: key = f.read()")
    print("  creds = grpc.ssl_server_credentials([(key, cert)], root_certificates=ca, require_client_auth=True)")
    print("  server.add_secure_port('[::]:50051', creds)")
    print("\n  # Client (Go):")
    print("  creds, _ := credentials.NewClientTLSFromFile('ca.crt', 'emotion-llm-service')")
    print("  // 或 mTLS:")
    print("  mtls, _ := credentials.NewMTLSFromFiles('ai-client.crt', 'ai-client.key', 'ca.crt', 'emotion-llm-service')")


if __name__ == "__main__":
    main()
