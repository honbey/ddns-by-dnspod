"""配置数据类"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import List


@dataclass
class DomainConfig:
    """单个域名的解析配置（可包含多个子域名）"""

    domain: str
    sub_domain: List[str] = field(default_factory=lambda: ["@"])
    record_type: str = "A"
    record_line: str = "默认"


@dataclass
class NtfyConfig:
    """ntfy 通知配置"""

    enabled: bool = False
    server: str = "https://ntfy.sh"
    topic: str = ""
    username: str = ""
    password: str = ""
    priority: int = 3
    tags: str = "ddns"


@dataclass
class AppConfig:
    """应用全局配置"""

    api_id: str = ""
    api_key: str = ""
    domains: List[DomainConfig] = field(default_factory=list)
    ip_check_urls: List[str] = field(
        default_factory=lambda: [
            "https://api.ipify.org",
            "http://ipv4.ip.sb",
            "http://api-ipv4.ip.sb",
            "https://v4.ident.me",
            "https://v4.tnedi.me",
            "https://ipv4.icanhazip.com",
        ]
    )
    db_path: str = "ddns_history.db"
    ntfy: NtfyConfig = field(default_factory=NtfyConfig)
