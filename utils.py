import json
import logging
import sys
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
        if record.exc_info:
            log_record["exception"] = self.formatException(record.exc_info)
        return json.dumps(log_record)


# class-like
def Logger(
    name: str, level: int = logging.INFO, format: Literal["json", "str"] = "json"
):
    if format == "json":
        formatter = JSONFormatter()
    else:
        formatter = logging.Formatter(
            "%(asctime)s - %(levelname)s - %(module)s:%(lineno)d - %(funcName)s() - %(message)s"
        )
    logger = logging.getLogger(name)
    handler = logging.StreamHandler(sys.stdout)
    handler.setLevel(level)
    handler.setFormatter(formatter)
    logger.addHandler(handler)
    return logger


def get_numeric_loglevel(level: str = "INFO") -> int:
    return getattr(logging, level.upper(), logging.INFO)


def push2gotify(
    title: str,
    msg: str,
    url: str,
    token: str,
    priority: int = 2,
    **kwargs,
):
    """
    Push notification to Gotify.

    Params:
    title (str): notification title
    message (str): notification message
    url (str): Gotify server URL
    token (str): Gotify token
    priority (int): notification priority
    **kwargs: other params to requests.post() except headers
    """

    url = f"{url}/message"
    headers = {"X-Gotify-Key": token, "Content-Type": "application/json"}
    extra_headers = kwargs.get("headers", None)
    if isinstance(extra_headers, dict):
        headers.update(extra_headers)
        kwargs.pop("headers", None)

    data = {"title": title, "message": msg, "priority": priority}

    logger = Logger("Gotify")

    try:
        response = requests.post(url, headers=headers, json=data, **kwargs)
        response.raise_for_status()
        logger.info("Push update notification to Gotify...")
    except requests.exceptions.RequestException as e:
        logger.error(f"Push notification to Gotify failed. error msg: {e}")
