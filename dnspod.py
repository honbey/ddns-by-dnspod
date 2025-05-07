import json

from tencentcloud.common.common_client import CommonClient
from tencentcloud.common import credential

from loguru import logger

from .iptools import judgeIp, getIp, getIpFromDNS

API_VERSION = "2021-03-23"
LOG_LEVEL = "DEBUG"

logger.add(
    "ddns_dev.log",
    level=LOG_LEVEL,
    rotation="10 MB",
    compression="gz",  # serialize=True
)
log = logger.bind(module_name="DDNS")


def first(n):
    return n[0] if isinstance(n, list) else n


def getv(d: dict, c: object, kw: str = ""):
    return d[kw] if d.get(kw, None) else first(getattr(c, kw, kw))


class DNSPodAPI:
    def __init__(self, kid, key, args):
        try:
            log.info("Initial DNSPodAPI begin.")
            self.args = args
            self.cred = credential.Credential(kid, key)
            self.common_client = CommonClient(
                "dnspod", API_VERSION, self.cred, "ap-shanghai"
            )
            log.debug(self.args)
        except Exception as e:
            log.error(e)
        finally:
            log.info("Initial DNSPodAPI end.")

    def info(self, **kargs):
        # data = {
        #    "Domain": kargs["domain"] if kargs["domain"] else first(self.args.domain)
        # }
        data = {"Domain": getv(kargs, self.args, "domain")}
        if kargs.get("subdomain", False):
            data.update({"Subdomain": kargs["subdomain"]})
        elif self.args.subdomain != "@":
            data.update({"Subdomain": first(self.args.subdomain)})
        log.debug(data)
        resp = self.req("DescribeRecordList", data)
        if isinstance(resp, dict):
            self.id = (
                resp.get("Response", {}).get("RecordList", [{}])[0].get("RecordId", 0)
            )
        else:
            self.id = -1
            log.error("cannot get record id")
        return resp

    def add_record(self, **kargs):
        data = {
            "Domain": getv(kargs, self.args, "domain"),
            "Value": getv(kargs, self.args, "ip"),
            "RecordType": getv(kargs, self.args, "type"),
            "RecordLine": getv(kargs, self.args, "line"),
        }
        if kargs.get("subdomain", False):
            data.update({"SubDomain": kargs["subdomain"]})
        elif self.args.subdomain != "@":
            data.update({"SubDomain": first(self.args.subdomain)})
        log.debug(data)
        if judgeIp(data["Value"]):
            self.req("CreateRecord", data)
        else:
            log.error("wrong IP address")

    def del_record(self, **kargs):
        domain = getv(kargs, self.args, "domain")
        subdomain = getv(kargs, self.args, "subdomain")
        self.info(domain=domain, subdomain=subdomain)
        if isinstance(self.id, int) and self.id != -1:
            data = {"Domain": domain, "RecordId": self.id}
            log.debug(data)
            self.req("DeleteRecord", data)
        else:
            log.error("delete record failure")

    def mod_record(self, ddns: bool = False, **kargs):
        domain = getv(kargs, self.args, "domain")
        subdomain = getv(kargs, self.args, "subdomain")
        data = {
            "Domain": domain,
            "SubDomain": subdomain,
            "RecordLine": getv(kargs, self.args, "line"),
        }
        if ddns:
            action = "ModifyDynamicDNS"
        else:
            data.update(
                {
                    "Value": getv(kargs, self.args, "ip"),
                    "RecordType": getv(kargs, self.args, "type"),
                }
            )
            action = "ModifyRecord"
        self.info(domain=domain, subdomain=subdomain)
        if isinstance(self.id, int) and self.id != -1:
            data.update({"RecordId": self.id})
            log.debug(data)
            self.req(action, data)
        else:
            log.error(f"{action} failure")

    def ddns(self, **kargs):
        domain = getv(kargs, self.args, "domain")
        subdomain = getv(kargs, self.args, "subdomain")
        resp = self.info(domain=domain, subdomain=subdomain)
        if isinstance(resp, dict):
            record_ip = str(
                resp.get("Response", {}).get("RecordList", [{}])[0].get("Value", "-1")
            )
        else:
            record_ip = "-1"
            log.error("cannot get record value")
        if not judgeIp(record_ip):
            log.error("cannot get record value")
            return
        dns_domain = domain if subdomain == "@" else subdomain + "." + domain
        url_ip = str(getIp())
        dns_ip = str(getIpFromDNS(dns_domain))
        log.info(f"url IP: [{url_ip}], dns IP: [{dns_ip}]")
        if judgeIp(url_ip) and url_ip == dns_ip:
            log.info(f"url IP[{url_ip}] is equal record IP[{dns_ip}], skipping ddns")
            return
        elif judgeIp(url_ip) and dns_ip == record_ip:
            log.info(f"dns IP[{dns_ip}] is equal record IP[{record_ip}], skipping ddns")
            return
        else:
            if isinstance(self.args.subdomain, list):
                subdomain_list = self.args.subdomain
                for sd in subdomain_list:
                    self.mod_record(ddns=True, domain=domain, subdomain=sd)
                    log.info(
                        f"updating record IP to [{url_ip}] for {sd + '.' + domain}"
                    )
            else:
                self.mod_record(ddns=True, domain=domain, subdomain=subdomain)
                log.info(
                    f"updating record IP to [{url_ip}] for {subdomain + '.' + domain}"
                )

    def req(self, action, data):
        try:
            log.debug(data)
            resp = self.common_client.call_json(action, data)
            log.debug(json.dumps(resp))
            return resp
        except Exception as e:
            log.error(e)
            return False
