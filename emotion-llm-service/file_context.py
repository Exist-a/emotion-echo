"""file_context.py · Stage 89 PR-4 — 附件文本抽取（文件理解发给 LLM）

只传 {url,name}（proto FileAttachment，字节不过 gRPC）；本模块负责：
1. URL 白名单校验（FILE_FETCH_ALLOWLIST，SSRF 防护；仅 http/https）
2. 限时下载（FILE_FETCH_TIMEOUT 秒；体积上限 FILE_FETCH_MAX_BYTES）
3. 按扩展名抽取文本：txt/md/csv/json/log → utf-8；pdf → pypdf；docx → python-docx
4. 截断（FILE_EXTRACT_MAX_CHARS/文件）

失败永不抛出——返回 (None, 原因)，由 build_file_context_text 注入"附件未能读取"注记。
（宽.fail 设计：文件上下文是增强，绝不阻断对话主链路。）
"""

import io
import logging
import os
import urllib.request
from dataclasses import dataclass, field

logger = logging.getLogger(__name__)

DEFAULT_ALLOWLIST = "emotion-echo-minio:9000,localhost:9000,127.0.0.1:9000"
DEFAULT_TIMEOUT_SECONDS = 5.0
DEFAULT_MAX_BYTES = 21 * 1024 * 1024  # 上传上限 20MiB + 余量
DEFAULT_MAX_CHARS = 6000

TEXT_EXTENSIONS = {".txt", ".md", ".csv", ".json", ".log"}


@dataclass
class FetchConfig:
    allowlist: set = field(default_factory=set)  # host[:port] 集合
    timeout: float = DEFAULT_TIMEOUT_SECONDS
    max_bytes: int = DEFAULT_MAX_BYTES
    max_chars: int = DEFAULT_MAX_CHARS


def resolve_fetch_config() -> FetchConfig:
    """从 env 解析配置；全部有默认值（dev 零配置可用）"""
    raw = os.environ.get("FILE_FETCH_ALLOWLIST", DEFAULT_ALLOWLIST)
    allowlist = {h.strip() for h in raw.split(",") if h.strip()}
    return FetchConfig(
        allowlist=allowlist,
        timeout=float(os.environ.get("FILE_FETCH_TIMEOUT", "") or DEFAULT_TIMEOUT_SECONDS),
        max_bytes=int(os.environ.get("FILE_FETCH_MAX_BYTES", "") or DEFAULT_MAX_BYTES),
        max_chars=int(os.environ.get("FILE_EXTRACT_MAX_CHARS", "") or DEFAULT_MAX_CHARS),
    )


def url_allowed(url: str, allowlist: set) -> bool:
    """URL 必须命中白名单 host[:port] 且 scheme 为 http/https（SSRF 防护）"""
    from urllib.parse import urlparse

    try:
        parsed = urlparse(url)
    except ValueError:
        return False
    if parsed.scheme not in ("http", "https"):
        return False
    netloc = parsed.netloc.lower()
    host = parsed.hostname
    if not host:
        return False
    if netloc in allowlist or host in allowlist:
        return True
    # 带默认端口的 URL（http=:80 / https=:443）与无端口白名单条目对齐
    if parsed.port is None and f"{host}:{'80' if parsed.scheme == 'http' else '443'}" in allowlist:
        return True
    return False


def _file_extension(name: str) -> str:
    dot = name.rfind(".")
    return name[dot:].lower() if dot >= 0 else ""


def extract_text_from_bytes(filename: str, data: bytes):
    """按扩展名抽取文本；不支持/失败返回 None（不抛出）"""
    ext = _file_extension(filename or "")
    try:
        if ext in TEXT_EXTENSIONS:
            return data.decode("utf-8", errors="replace")
        if ext == ".pdf":
            return _extract_pdf(data)
        if ext == ".docx":
            return _extract_docx(data)
        return None
    except Exception as e:
        logger.warning(f"[file_context] extract failed for {filename!r}: {e}")
        return None


def _extract_pdf(data: bytes):
    """pypdf 按页抽取；延迟 import（未安装时按不支持处理）"""
    try:
        from pypdf import PdfReader
    except ImportError:
        logger.warning("[file_context] pypdf not installed, pdf skipped")
        return None
    reader = PdfReader(io.BytesIO(data))
    pages = [(page.extract_text() or "") for page in reader.pages]
    text = "\n".join(p for p in pages if p.strip())
    return text or None


def _extract_docx(data: bytes):
    """python-docx 抽取段落文本；延迟 import"""
    try:
        import docx
    except ImportError:
        logger.warning("[file_context] python-docx not installed, docx skipped")
        return None
    document = docx.Document(io.BytesIO(data))
    parts = [p.text for p in document.paragraphs if p.text.strip()]
    for table in document.tables:
        for row in table.rows:
            parts.append("\t".join(cell.text.strip() for cell in row.cells))
    text = "\n".join(parts)
    return text or None


def truncate_text(text: str, max_chars: int) -> str:
    """超限头部截断并加标记（DeepSeek 上下文有限；单文件不挤占对话预算）"""
    if len(text) <= max_chars:
        return text
    return text[:max_chars] + f"\n…[附件内容过长，已截断，原始 {len(text)} 字符]"


def fetch_and_extract(url: str, name: str, cfg: FetchConfig | None = None):
    """下载 + 抽取；返回 (text|None, reason)。失败永不抛出"""
    if cfg is None:
        cfg = resolve_fetch_config()
    if not url_allowed(url, cfg.allowlist):
        return None, f"url 未命中白名单: {url}"
    try:
        request = urllib.request.Request(url, method="GET")
        with urllib.request.urlopen(request, timeout=cfg.timeout) as resp:
            if resp.status != 200:
                return None, f"下载失败 HTTP {resp.status}"
            data = resp.read(cfg.max_bytes + 1)
            if len(data) > cfg.max_bytes:
                return None, f"文件过大（>{cfg.max_bytes} 字节），拒绝抽取"
    except Exception as e:
        return None, f"下载失败: {e}"
    text = extract_text_from_bytes(name, data)
    if text is None:
        return None, "不支持的文件类型或内容抽取失败"
    return truncate_text(text, cfg.max_chars), ""


def build_file_context_text(attachments, cfg: FetchConfig | None = None) -> str:
    """把附件列表转为 system 上下文块；全部失败/无附件返回 ''

    attachments: [{"url": str, "name": str}]（proto FileAttachment 的 dict 形状）
    """
    if not attachments:
        return ""
    if cfg is None:
        cfg = resolve_fetch_config()
    sections = []
    for i, att in enumerate(attachments, start=1):
        name = (att.get("name") or att.get("url", "").rsplit("/", 1)[-1]) or f"attachment-{i}"
        text, reason = fetch_and_extract(att.get("url", ""), name, cfg)
        if text is not None:
            sections.append(f"--- 附件 {i}：{name} ---\n{text}")
        else:
            sections.append(f"--- 附件 {i}：{name}（未能读取：{reason}）---")
    return (
        "[附件上下文]\n"
        "以下内容来自用户上传的附件，回答用户问题时可参考；"
        "若附件内容与问题无关请如实说明：\n" + "\n".join(sections)
    )
