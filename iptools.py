import ipaddress
import socket
import requests

from dns import resolver

import yaml

# Force IPv4
# https://stackoverflow.com/a/50044152
__old_getaddrinfo = socket.getaddrinfo


def __new_getaddrinfo(*args, **kwargs):
    responses = __old_getaddrinfo(*args, **kwargs)
    return [response for response in responses if response[0] == socket.AF_INET]


socket.getaddrinfo = __new_getaddrinfo


USER_AGENT = "curl/8.5.0"
TIMEOUT = (15, 15)


class IpInfo:
    def __init__(self) -> None:
        with open("ip.yaml", "r") as f:
            config = yaml.safe_load(f)
            self.common_api_pool = config["ip_api"]
            self.json_api_pool = config["ip_api_json"]

            self.dns = resolver.Resolver()
            self.dns.nameservers = config["dns_server"]

    @staticmethod
    def judge_ip(ip):
        try:
            return ipaddress.ip_address(ip).is_global
        except ValueError:
            return False

    def _get_ip_common(self):
        for url in self.common_api_pool:
            resp = requests.get(
                url=url,
                headers={"User-Agent": USER_AGENT, "Accept": "*/*"},
                timeout=TIMEOUT,
            )
            resp.encoding = "UTF-8"
            ip = resp.text.replace("\n", "")
            if self.judge_ip(ip):
                return ip
            else:
                continue
        return False

    def _get_ip_from_json(self):
        for api in self.json_api_pool:
            try:
                resp = requests.get(
                    url=api["url"],
                    headers={"User-Agent": USER_AGENT, "Accept": "*/*"},
                    timeout=TIMEOUT,
                )
                resp.encoding = "UTF-8"
                ip = resp.json()
                for key in api["path"]:
                    ip = ip.get(key)
                if self.judge_ip(ip):
                    return ip
                else:
                    continue
            except Exception:
                continue
        return False

    def dns_resolve(self, domain: str, type="A"):
        ip = self.dns.resolve(domain, type)
        if self.judge_ip(str(ip[0])):
            return ip[0]
        else:
            return False

    def get_ip(self):
        return self._get_ip_common() if self._get_ip_common else self._get_ip_from_json


if __name__ == "__main__":
    tool = IpInfo()
    print(tool.get_ip())
    print(tool.dns_resolve("example.com"))
