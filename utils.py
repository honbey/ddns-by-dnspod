import logging
import json
from datetime import datetime
from typing import Literal

import requests


def first(n):
    return n[0] if isinstance(n, list) else n


def getv(d: dict, c: object, kw: str = ""):
    return d[kw] if d.get(kw, False) else first(getattr(c, kw, ""))


class JSONFormatter(logging.Formatter):
    def format(self, record):
        log_record = {
            "timestamp": datetime.now().isoformat(),
            "level": record.levelname,
            "name": record.name,
            "message": record.getMessage(),
            "module": record.module,
            "function": record.funcName,
            "line": record.lineno,
            "thread": record.threadName,
        }
        # 添加异常信息（如果有）
        if record.exc_info:
            log_record["exception"] = self.formatException(record.exc_info)
        return json.dumps(log_record)


# class-like
def Logger(
    name: str, level: int = logging.INFO, format: Literal["json", "str"] = "json"
):
    logger = logging.getLogger(name)
    logger.setLevel(level)
    if format == "json":
        handler = logging.StreamHandler()
        handler.setFormatter(JSONFormatter())
        logger.addHandler(handler)
    else:
        logging.basicConfig(
            format="%(asctime)s - %(levelname)s - %(module)s:%(lineno)d - %(funcName)s() - %(message)s",
            datefmt="%Y-%m-%d %H:%M:%S",
        )
    return logger


def push2gotify(
    title: str,
    msg: str,
    url: str,
    token: str,
    headers: dict = {},
    verify: bool = True,
    priority: int = 2,
):
    """
    Push notification to Gotify.

    Params:
    title (str): notification title
    message (str): notification message
    url (str): Gotify server URL
    token (str): Gotify token
    priority (int): notification priority
    """

    url = f"{url}/message"
    headers.update({"X-Gotify-Key": token, "Content-Type": "application/json"})
    data = {"title": title, "message": msg, "priority": priority}

    logger = Logger("Gotify")

    try:
        response = requests.post(url, headers=headers, json=data, verify=verify)
        response.raise_for_status()
        logger.info("Push update notification to Gotify...")
    except requests.exceptions.RequestException as e:
        logger.error(f"Push notification to Gotify failed. error msg: {e}")
