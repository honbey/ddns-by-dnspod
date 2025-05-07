import sys
import json

from tencentcloud.common.common_client import CommonClient
from tencentcloud.common import credential

from loguru import logger

from iptools import judgeIp, getIp, getIpFromDNS

API_VERSION = "2021-03-23"


def first(n):
    return n[0] if isinstance(n, list) else n


def getv(d: dict, c: object, kw: str = ""):
    return d[kw] if d.get(kw, None) else first(getattr(c, kw, None))


class DNSPodAPI:
    def __init__(self, kid, key, args):
        try:
            self.args = args
            self.log = self.logger()
            self.log.debug("Initial DNSPodAPI begin.")
            self.cred = credential.Credential(kid, key)
            self.common_client = CommonClient(
                "dnspod", API_VERSION, self.cred, "ap-shanghai"
            )
            self.log.debug(self.args)
        except Exception as e:
            self.log.error(e)
        finally:
            self.log.debug("Initial DNSPodAPI end.")

    def logger(self, **kwargs):
        if not getv(kwargs, self.args, "debug"):
            logger.remove(handler_id=None)
        file = getv(kwargs, self.args, "log_file")
        level = getv(kwargs, self.args, "log_level") or "INFO"
        size = getv(kwargs, self.args, "log_size") or 10
        if isinstance(file, str):
            logger.add(
                file,
                level=level,
                rotation=f"{size} MB",
                compression="gz",  # serialize=True
            )
        else:
            logger.add(sys.stdout, level=level)
        return logger.bind(module_name="DDNS by DNSPod")

    def info(self, **kwargs):
        # data = {
        #    "Domain": kwargs["domain"] if kargs["domain"] else first(self.args.domain)
        # }
        data = {"Domain": getv(kwargs, self.args, "domain")}
        if kwargs.get("subdomain", False):
            data.update({"Subdomain": kwargs["subdomain"]})
        elif self.args.subdomain != "@":
            data.update({"Subdomain": first(self.args.subdomain)})
        self.log.debug(data)
        resp = self.req("DescribeRecordList", data)
        if isinstance(resp, dict):
            self.id = (
                resp.get("Response", {}).get("RecordList", [{}])[0].get("RecordId", 0)
            )
        else:
            self.id = -1
            self.log.error("cannot get record id")
        return resp

    def add_record(self, **kwargs):
        data = {
            "Domain": getv(kwargs, self.args, "domain"),
            "Value": getv(kwargs, self.args, "ip"),
            "RecordType": getv(kwargs, self.args, "type"),
            "RecordLine": getv(kwargs, self.args, "line"),
        }
        if kwargs.get("subdomain", False):
            data.update({"SubDomain": kwargs["subdomain"]})
        elif self.args.subdomain != "@":
            data.update({"SubDomain": first(self.args.subdomain)})
        self.log.debug(data)
        if judgeIp(data["Value"]):
            self.req("CreateRecord", data)
        else:
            self.log.error("wrong IP address")

    def del_record(self, **kwargs):
        domain = getv(kwargs, self.args, "domain")
        subdomain = getv(kwargs, self.args, "subdomain")
        self.info(domain=domain, subdomain=subdomain)
        if isinstance(self.id, int) and self.id != -1:
            data = {"Domain": domain, "RecordId": self.id}
            self.log.debug(data)
            self.req("DeleteRecord", data)
        else:
            self.log.error("delete record failure")

    def mod_record(self, ddns: bool = False, **kwargs):
        domain = getv(kwargs, self.args, "domain")
        subdomain = getv(kwargs, self.args, "subdomain")
        data = {
            "Domain": domain,
            "SubDomain": subdomain,
            "RecordLine": getv(kwargs, self.args, "line"),
        }
        if ddns:
            action = "ModifyDynamicDNS"
        else:
            data.update(
                {
                    "Value": getv(kwargs, self.args, "ip"),
                    "RecordType": getv(kwargs, self.args, "type"),
                }
            )
            action = "ModifyRecord"
        self.info(domain=domain, subdomain=subdomain)
        if isinstance(self.id, int) and self.id != -1:
            data.update({"RecordId": self.id})
            self.log.debug(data)
            self.req(action, data)
        else:
            self.log.error(f"{action} failure")

    def ddns(self, **kwargs) -> str:
        domain = getv(kwargs, self.args, "domain")
        subdomain = getv(kwargs, self.args, "subdomain")
        resp = self.info(domain=domain, subdomain=subdomain)
        if isinstance(resp, dict):
            record_ip = str(
                resp.get("Response", {}).get("RecordList", [{}])[0].get("Value", "-1")
            )
        else:
            record_ip = "-1"
            self.log.error("cannot get record value")
        if not judgeIp(record_ip):
            self.log.error("cannot get record value")
            return
        dns_domain = domain if subdomain == "@" else subdomain + "." + domain
        url_ip = str(getIp())
        dns_ip = str(getIpFromDNS(dns_domain))
        self.log.info(f"url IP: [{url_ip}], dns IP: [{dns_ip}]")
        if judgeIp(url_ip) and url_ip == dns_ip:
            self.log.info(
                f"url IP[{url_ip}] is equal record IP[{dns_ip}], skipping ddns"
            )
            return ""
        elif judgeIp(url_ip) and url_ip == record_ip:
            self.log.info(
                f"dns IP[{url_ip}] is equal record IP[{record_ip}], skipping ddns"
            )
            return ""
        else:
            if isinstance(self.args.subdomain, list):
                subdomain_list = self.args.subdomain
                for sd in subdomain_list:
                    self.mod_record(ddns=True, domain=domain, subdomain=sd)
                    self.log.info(
                        f"updating record IP to [{url_ip}] for {sd + '.' + domain}"
                    )
            else:
                self.mod_record(ddns=True, domain=domain, subdomain=subdomain)
                self.log.info(
                    f"updating record IP to [{url_ip}] for {subdomain + '.' + domain}"
                )
            return f"Successfully updated {record_ip} to {url_ip}."

    def req(self, action, data):
        try:
            self.log.debug(data)
            resp = self.common_client.call_json(action, data)
            self.log.debug(json.dumps(resp))
            return resp
        except Exception as e:
            self.log.error(e)
            return False
