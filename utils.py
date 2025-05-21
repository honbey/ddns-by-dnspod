
def first(n):
    return n[0] if isinstance(n, list) else n

def getv(d: dict, c: object, kw: str = ""):
    return d[kw] if d.get(kw, False) else first(getattr(c, kw, ""))
