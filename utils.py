import logging
from typing import Literal


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
        level=level,
        format=fmt,
        datefmt="%Y-%m-%d %H:%M:%S",
    )
    return logging.getLogger(name)
