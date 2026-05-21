"""ntfy 通知模块（支持用户认证）"""

from __future__ import annotations

import requests

from .config import NtfyConfig


def send_notification(ntfy: NtfyConfig, title: str, message: str) -> bool:
    """发送 ntfy 通知，返回是否成功。"""
    if not ntfy.enabled or not ntfy.topic:
        return False

    url = f"{ntfy.server.rstrip('/')}/{ntfy.topic}"

    headers = {
        "Title": title,
        "Priority": str(ntfy.priority),
        "Tags": ntfy.tags,
    }

    auth = None
    if ntfy.username and ntfy.password:
        auth = (ntfy.username, ntfy.password)

    try:
        resp = requests.post(
            url,
            data=message.encode("utf-8"),
            headers=headers,
            auth=auth,
            timeout=10,
        )
        return resp.status_code == 200
    except requests.RequestException:
        return False
