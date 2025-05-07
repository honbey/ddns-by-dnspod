import ipaddress
import socket
import requests

from dns import resolver

import yaml

# https://stackoverflow.com/a/50044152
old_getaddrinfo = socket.getaddrinfo


def new_getaddrinfo(*args, **kwargs):
    responses = old_getaddrinfo(*args, **kwargs)
    return [response for response in responses if response[0] == socket.AF_INET]


socket.getaddrinfo = new_getaddrinfo


user_agent = "curl/8.5.0"
timeout = (15, 15)

dns = resolver.Resolver()

with open("ip.yaml", "r") as f:
    config = yaml.safe_load(f)
    common_api_pool = config["ip_api"]
    json_api_pool = config["ip_api_json"]
    dns.nameservers = config["dns_server"]


def judgeIp(ip: str) -> bool:
    try:
        return ipaddress.ip_address(ip).is_global
    except ValueError:
        return False


def getIpCommon():
    for url in common_api_pool:
        resp = requests.get(
            url=url,
            headers={"User-Agent": user_agent, "Accept": "*/*"},
            timeout=timeout,
        )
        resp.encoding = "UTF-8"
        ip = resp.text.replace("\n", "")
        if judgeIp(ip):
            return ip
        else:
            continue
    return False


def getIpFromJSON():
    for api in json_api_pool:
        try:
            resp = requests.get(
                url=api["url"],
                headers={"User-Agent": user_agent, "Accept": "*/*"},
                timeout=timeout,
            )
            resp.encoding = "UTF-8"
            ip = resp.json()
            for key in api["path"]:
                ip = ip.get(key)
            if judgeIp(ip):
                return ip
            else:
                continue
        except Exception:
            continue
    return False


def getIpFromDNS(domain: str, type="A"):
    ip = dns.resolve(domain, type)
    if judgeIp(str(ip[0])):
        return ip[0]
    else:
        return False


def getIp():
    return getIpCommon() if getIpCommon() else getIpFromJSON()


if __name__ == "__main__":
    # print(config)
    print(getIp())
    print(getIpFromDNS("example.com"))
