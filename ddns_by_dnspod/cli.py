"""命令行参数解析与 YAML 配置加载"""

from __future__ import annotations

import argparse
import os
import sys
from typing import List, Union

import yaml

from .config import AppConfig, DomainConfig, NtfyConfig


def _normalize_sub_domains(value: Union[str, List[str], None]) -> List[str]:
    """将字符串或列表统一转为子域名列表。"""
    if value is None:
        return ["@"]
    if isinstance(value, list):
        return [str(s) for s in value if s]
    if isinstance(value, str):
        # 支持逗号分隔
        parts = [s.strip() for s in value.split(",") if s.strip()]
        return parts if parts else ["@"]
    return ["@"]


def _dict_to_config(data: dict) -> AppConfig:
    """将 YAML 字典转换为 AppConfig 对象。"""
    config = AppConfig()

    config.secret_id = data.get("secret_id", "")
    config.secret_key = data.get("secret_key", "")
    config.db_path = data.get("db_path", "ddns_history.db")

    # IP 检测地址列表
    urls = data.get("ip_check_urls")
    if urls and isinstance(urls, list):
        config.ip_check_urls = urls
    elif data.get("ip_check_url"):  # 兼容旧版单地址
        config.ip_check_urls = [data["ip_check_url"]]

    # 解析域名列表
    for d in data.get("domains", []):
        sub = _normalize_sub_domains(d.get("sub_domain", "@"))
        config.domains.append(
            DomainConfig(
                domain=d.get("domain", ""),
                sub_domain=sub,
                record_type=d.get("record_type", "A"),
                record_line=d.get("record_line", "默认"),
            )
        )

    # 解析 ntfy 配置
    ntfy_raw = data.get("ntfy", {})
    config.ntfy = NtfyConfig(
        enabled=ntfy_raw.get("enabled", False),
        server=ntfy_raw.get("server", "https://ntfy.sh"),
        topic=ntfy_raw.get("topic", ""),
        username=ntfy_raw.get("username", ""),
        password=ntfy_raw.get("password", ""),
        priority=ntfy_raw.get("priority", 3),
        tags=ntfy_raw.get("tags", "ddns"),
    )

    return config


def _override_from_args(config: AppConfig, args: argparse.Namespace) -> AppConfig:
    """命令行参数覆盖 YAML 配置（命令行参数优先级更高）。"""
    if args.secret_id:
        config.secret_id = args.secret_id
    if args.secret_key:
        config.secret_key = args.secret_key

    # 域名相关
    if args.domain:
        sub = _normalize_sub_domains(args.sub_domain if args.sub_domain else "@")
        config.domains = [
            DomainConfig(
                domain=args.domain,
                sub_domain=sub,
                record_type=args.record_type or "A",
                record_line=args.record_line or "默认",
            )
        ]
    else:
        # 如果没有通过 -d 指定域名，但提供了子域名等其他参数，修改第一个域名的配置
        if config.domains and args.sub_domain:
            config.domains[0].sub_domain = _normalize_sub_domains(args.sub_domain)
        if config.domains and args.record_type:
            config.domains[0].record_type = args.record_type
        if config.domains and args.record_line:
            config.domains[0].record_line = args.record_line

    if args.ip_check_urls:
        config.ip_check_urls = args.ip_check_urls
    if args.db_path:
        config.db_path = args.db_path

    # ntfy
    if args.ntfy_topic:
        config.ntfy.enabled = True
        config.ntfy.topic = args.ntfy_topic
    if args.ntfy_server:
        config.ntfy.server = args.ntfy_server
    if args.ntfy_user:
        config.ntfy.username = args.ntfy_user
    if args.ntfy_pass:
        config.ntfy.password = args.ntfy_pass
    if args.ntfy_priority is not None:
        config.ntfy.priority = args.ntfy_priority

    return config


def build_parser() -> argparse.ArgumentParser:
    """构建命令行参数解析器。"""
    parser = argparse.ArgumentParser(
        prog="dnspod-ddns",
        description="动态域名解析更新工具，基于腾讯云 DNSPod API 3.0",
    )

    # 配置文件
    parser.add_argument(
        "-c",
        "--config",
        type=str,
        default=None,
        help="YAML 配置文件路径（默认: 当前目录下的 config.yml）",
    )

    # 腾讯云凭证
    parser.add_argument("--secret-id", type=str, help="腾讯云 SecretId")
    parser.add_argument("--secret-key", type=str, help="腾讯云 SecretKey")

    # 域名配置
    parser.add_argument("-d", "--domain", type=str, help="主域名，如 example.com")
    parser.add_argument(
        "-s",
        "--sub-domain",
        type=str,
        action="append",
        default=None,
        help="主机记录，可多次使用，如 -s @ -s www（覆盖配置文件中的列表）",
    )
    parser.add_argument(
        "-t", "--record-type", type=str, choices=["A", "AAAA"], help="记录类型"
    )
    parser.add_argument("-l", "--record-line", type=str, help="记录线路（默认: 默认）")

    # IP 检测
    parser.add_argument(
        "--ip-check-urls",
        nargs="+",
        help="公网 IP 检测服务地址列表（覆盖配置文件中的列表）",
    )

    # SQLite
    parser.add_argument("--db-path", type=str, help="SQLite 数据库文件路径")

    # ntfy
    parser.add_argument("--ntfy-topic", type=str, help="ntfy 通知主题")
    parser.add_argument("--ntfy-server", type=str, help="ntfy 服务器地址")
    parser.add_argument("--ntfy-user", type=str, help="ntfy 认证用户名")
    parser.add_argument("--ntfy-pass", type=str, help="ntfy 认证密码")
    parser.add_argument(
        "--ntfy-priority",
        type=int,
        choices=[1, 2, 3, 4, 5],
        help="ntfy 通知优先级（1-5）",
    )

    # 只打印当前 IP，不更新
    parser.add_argument(
        "--show-ip", action="store_true", help="仅显示当前公网 IP，不执行更新"
    )

    return parser


def load_config(args: argparse.Namespace) -> AppConfig:
    """加载配置：先读 YAML，再用命令行参数覆盖，最后用环境变量兜底。"""
    config_path = args.config

    # 未指定配置文件时，尝试默认路径
    if config_path is None:
        for p in ["config.yml", "config.yaml"]:
            if os.path.isfile(p):
                config_path = p
                break

    # 读取 YAML
    if config_path and os.path.isfile(config_path):
        with open(config_path, "r", encoding="utf-8") as f:
            data = yaml.safe_load(f) or {}
        config = _dict_to_config(data)
    else:
        config = AppConfig()

    # 命令行参数覆盖
    config = _override_from_args(config, args)

    # 环境变量兜底（优先级最高）
    config.secret_id = os.environ.get("TENCENTCLOUD_API_ID", config.secret_id)
    config.secret_key = os.environ.get("TENCENTCLOUD_API_KEY", config.secret_key)

    return config


def main() -> None:
    """程序入口。"""
    parser = build_parser()
    args = parser.parse_args()

    config = load_config(args)

    # 校验必要配置
    if not args.show_ip:
        errors = []
        if not config.secret_id:
            errors.append(
                "缺少 secret_id（可通过 --secret-id、YAML 或环境变量 TENCENTCLOUD_API_ID 提供）"
            )
        if not config.secret_key:
            errors.append(
                "缺少 secret_key（可通过 --secret-key、YAML 或环境变量 TENCENTCLOUD_API_KEY 提供）"
            )
        if not config.domains:
            errors.append("缺少域名配置（可通过 --domain、YAML 提供）")
        if errors:
            print("配置错误:", file=sys.stderr)
            for e in errors:
                print(f"  - {e}", file=sys.stderr)
            sys.exit(1)

    # 延迟导入，加快 --help 响应速度
    from .ddns import run_ddns

    run_ddns(config, show_ip_only=args.show_ip)
