"""核心 DDNS 逻辑：获取公网 IP、查询/更新 DNSPod 记录"""

from __future__ import annotations

import sys
from datetime import datetime, timezone
from typing import List, Optional, Tuple

import requests
from tencentcloud.common import credential
from tencentcloud.common.exception.tencent_cloud_sdk_exception import (
    TencentCloudSDKException,
)
from tencentcloud.dnspod.v20210323 import dnspod_client, models

from .config import AppConfig
from .db import (
    init_db,
    get_last_public_ip,
    set_last_public_ip,
    insert_record,
)
from .notify import send_notification


def _get_public_ip(urls: List[str]) -> Optional[str]:
    """尝试多个检测地址获取公网 IP，返回第一个成功的结果。"""
    for url in urls:
        try:
            resp = requests.get(url, timeout=10)
            resp.raise_for_status()
            ip = resp.text.strip()
            if ip:
                return ip
        except requests.RequestException:
            continue
    print("[ERROR] 所有公网 IP 检测地址均失败", file=sys.stderr)
    return None


def _create_dnspod_client(config: AppConfig) -> dnspod_client.DnspodClient:
    """创建腾讯云 DNSPod 客户端。"""
    cred = credential.Credential(config.secret_id, config.secret_key)
    return dnspod_client.DnspodClient(cred, "")


def _get_record_list(
    client: dnspod_client.DnspodClient,
    domain: str,
    sub_domain: str,
    record_type: str,
) -> list:
    """查询域名下指定主机记录和类型的解析记录列表。"""
    req = models.DescribeRecordListRequest()
    req.Domain = domain
    req.Subdomain = sub_domain
    req.RecordType = record_type

    try:
        resp = client.DescribeRecordList(req)
        return resp.RecordList or []
    except TencentCloudSDKException as e:
        print(f"[ERROR] 查询记录失败 ({domain}/{sub_domain}): {e}", file=sys.stderr)
        return []


def _modify_record(
    client: dnspod_client.DnspodClient,
    domain: str,
    sub_domain: str,
    record_type: str,
    record_line: str,
    record_id: int,
    value: str,
) -> Tuple[bool, str]:
    """修改一条解析记录。返回 (是否成功, 消息)。"""
    req = models.ModifyRecordRequest()
    req.Domain = domain
    req.SubDomain = sub_domain
    req.RecordType = record_type
    req.RecordLine = record_line
    req.RecordId = record_id
    req.Value = value

    try:
        resp = client.ModifyRecord(req)
        return True, f"记录 {resp.RecordId} 已更新为 {value}"
    except TencentCloudSDKException as e:
        return False, str(e)


def _compute_duration(
    conn,
    domain: str,
    sub_domain: str,
    record_type: str,
    old_ip: str,
) -> Optional[int]:
    """
    计算旧 IP 的持续时间（秒）。
    逻辑：查找该记录最近一次操作的时间，且那次操作的 new_ip 等于当前 old_ip。
    """
    row = conn.execute(
        """SELECT created_at FROM ddns_history
           WHERE domain = ? AND sub_domain = ? AND record_type = ?
             AND new_ip = ?
           ORDER BY id DESC LIMIT 1""",
        (domain, sub_domain, record_type, old_ip),
    ).fetchone()
    if not row:
        # 可能是第一次出现，或者 IP 记录不匹配，持续时间为 0 或 None
        return None
    try:
        last_dt = datetime.fromisoformat(row[0])
        now_dt = datetime.now(timezone.utc)
        return int((now_dt - last_dt).total_seconds())
    except (ValueError, OSError):
        return None


def _update_single_domain(
    client: dnspod_client.DnspodClient,
    ip: str,
    domain: str,
    sub_domain: str,
    record_type: str,
    record_line: str,
    conn,
    ntfy_cfg,
) -> bool:
    """
    更新单个子域名记录。
    返回 True 表示发生了实际更新（用于汇总通知）。
    """
    records = _get_record_list(client, domain, sub_domain, record_type)

    if not records:
        msg = "未找到匹配的记录"
        print(f"[WARN] {domain}/{sub_domain}: {msg}")
        insert_record(
            conn,
            domain,
            sub_domain,
            record_type,
            None,
            None,
            ip,
            "SKIP",
            msg,
            duration=None,
        )
        return False

    updated = False
    for rec in records:
        old_ip = rec.Value
        if old_ip == ip:
            # IP 未变，计算持续时间
            duration = _compute_duration(conn, domain, sub_domain, record_type, old_ip)
            print(f"[INFO] {domain}/{sub_domain}: IP 未变化 ({ip})，跳过")
            insert_record(
                conn,
                domain,
                sub_domain,
                record_type,
                rec.RecordId,
                old_ip,
                ip,
                "SKIP",
                "IP 未变化",
                duration=duration,
            )
            continue

        # 执行更新前先计算旧 IP 持续时间
        duration = _compute_duration(conn, domain, sub_domain, record_type, old_ip)
        success, msg = _modify_record(
            client,
            domain,
            sub_domain,
            record_type,
            record_line,
            rec.RecordId,
            ip,
        )
        status = "OK" if success else "FAIL"
        print(f"[{status}] {domain}/{sub_domain}: {msg}")
        insert_record(
            conn,
            domain,
            sub_domain,
            record_type,
            rec.RecordId,
            old_ip,
            ip,
            status,
            msg,
            duration=duration,
        )
        if success:
            updated = True

    return updated


def run_ddns(config: AppConfig, show_ip_only: bool = False) -> None:
    """运行 DDNS 主流程。"""
    # 获取公网 IP
    ip = _get_public_ip(config.ip_check_urls)
    if ip is None:
        sys.exit(1)

    if show_ip_only:
        print(ip)
        return

    current_time = datetime.now()
    formatted_time = current_time.strftime("%Y-%m-%d %H:%M:%S")

    print(f"[INFO] {formatted_time}: 当前公网 IP: {ip}")

    # 初始化数据库
    conn = init_db(config.db_path)

    # 检查全局 IP 是否变化（避免不必要的 API 调用）
    last_ip = get_last_public_ip(conn)
    if last_ip == ip:
        print("[INFO] 公网 IP 与上次相同，无需更新，程序退出。")
        conn.close()
        return

    # IP 已变化，更新状态
    set_last_public_ip(conn, ip)

    # 创建 DNSPod 客户端
    client = _create_dnspod_client(config)

    # 遍历所有域名配置，展开子域名列表
    any_updated = False
    for domain_cfg in config.domains:
        for sub in domain_cfg.sub_domain:
            sub = sub.strip()
            if not sub:
                sub = "@"
            updated = _update_single_domain(
                client,
                ip,
                domain_cfg.domain,
                sub,
                domain_cfg.record_type,
                domain_cfg.record_line,
                conn,
                config.ntfy,
            )
            if updated:
                any_updated = True

    # 整体通知（仅当有实际更新时发送一次）
    if any_updated and config.ntfy.enabled:
        send_notification(
            config.ntfy,
            title="DNSPod DDNS 更新",
            message=f"公网 IP 已变更为 {ip}，相关域名记录已更新。",
        )

    conn.close()
    print("[INFO] 所有域名处理完毕")
