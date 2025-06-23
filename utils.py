import logging
from typing import Literal

import requests


def first(n):
    return n[0] if isinstance(n, list) else n


def getv(d: dict, c: object, kw: str = ""):
    return d[kw] if d.get(kw, False) else first(getattr(c, kw, ""))


def Logger(
    name: str, level: int = logging.INFO, format: Literal["json", "str"] = "json"
):
    if format == "json":
        fmt = '{"time": "%(asctime)s", "level": "%(levelname)s", "line_number": %(lineno)s, "function_name": "%(funcName)s()", "message": "%(message)s"}'
    else:
        fmt = "%(asctime)s - %(levelname)s - %(filename)s:%(lineno)d - %(funcName)s() - %(message)s"
    logging.basicConfig(
        format=fmt,
        datefmt="%Y-%m-%d %H:%M:%S",
    )
    logger = logging.getLogger(name)
    logger.setLevel(level)
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
