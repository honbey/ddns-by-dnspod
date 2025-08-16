import ipaddress

import requests
import yaml
from dns import resolver
from utils import Log

USER_AGENT = "curl/8.5.0"
TIMEOUT = (15, 15)


class IpInfo:
    log = Log("Ip Tools").logger

    def __init__(self, config="ip.yaml", version: int = 4) -> None:
        with open(config, "r") as f:
            config = yaml.safe_load(f)
            if version == 4:
                self._common_api_pool = config["ipv4_api"]
                self._json_api_pool = config["ipv4_api_json"]
            elif version == 6:
                self._common_api_pool = config["ipv6_api"]
                self._json_api_pool = config["ipv6_api_json"]
            else:
                IpInfo.log.error("Wrong version, only support 4(IPv4) or 6(IPv6).")

            self._dns = resolver.Resolver()
            self._dns.nameservers = config["dns_server"]

    @staticmethod
    def judge_ip(ip):
        try:
            return ipaddress.ip_address(ip).is_global
        except ValueError as e:
            IpInfo.log.error(f"[{ip}] is wrong IP address.")
            IpInfo.log.debug("ValueError: %s", str(e), exc_info=True)
            return False

    def _get_ip_common(self):
        for url in self._common_api_pool:
            try:
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
            except Exception as e:
                IpInfo.log.error("Catched an exception!")
                IpInfo.log.debug("Exception: %s", str(e), exc_info=True)
                continue

        return False

    def _get_ip_from_json(self):
        for api in self._json_api_pool:
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
            except Exception as e:
                IpInfo.log.error("Catched an exception!")
                IpInfo.log.debug("Exception: %s", str(e), exc_info=True)
                continue
        return False

    def dns_resolve(self, domain: str, type: str = "A"):
        try:
            ip = self._dns.resolve(domain, type)
            if self.judge_ip(str(ip[0])):
                return ip[0]
            else:
                return False
        except Exception as e:
            IpInfo.log.error("Catched an exception!")
            IpInfo.log.debug("Exception: %s", str(e), exc_info=True)
            return False

    def get_ip(self):
        return self._get_ip_common() if self._get_ip_common else self._get_ip_from_json


if __name__ == "__main__":
    tool = IpInfo()
    print(tool.get_ip())
    print(tool.dns_resolve("example.com"))
