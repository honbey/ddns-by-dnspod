#!/usr/bin/env python3
# _*_ coding:utf-8 _*_
#

import logging

from db import DomainDatabase
from dnspod import DNSPodAPI
from iptools import IpInfo
from utils import Logger

LOG_LEVEL = logging.INFO

logger = Logger("DDNS by DNSPod", level=LOG_LEVEL)


def ddns(force_update: bool = False) -> bool:
    db = DomainDatabase(
        db="/opt/data/workspace/ddns-by-dnspod/data.db", log_level=LOG_LEVEL
    )
    record_list = db.query_record_by_group("DDNS")
    one_record = record_list[0]
    record_ip = one_record.get("value", "0.0.0.0")
    domain = one_record.get("domain", "example.com")
    subdomain = one_record.get("subdomain", "@")
    fulldomain = domain if subdomain == "@" else subdomain + "." + domain
    info = IpInfo(config="/opt/data/workspace/ddns-by-dnspod/ip.yaml")
    url_ip = str(info.get_ip())
    dns_ip = str(info.dns_resolve(fulldomain))
    if force_update:
        record_ip, dns_ip = ["", ""]
    logger.info(f"URL IP: [{url_ip}], DNS IP: [{dns_ip}], record IP: [{record_ip}]")
    if IpInfo.judge_ip(url_ip) and url_ip == dns_ip:
        logger.info("URL IP is equal to DNS IP, skipping DDNS.")
        return False
    elif IpInfo.judge_ip(url_ip) and url_ip == record_ip:
        logger.info("URL IP is equal to record IP, skipping DDNS.")
        return False
    else:
        dnspod = DNSPodAPI(db.key, log_level=logging.INFO)
        for record in record_list:
            subdomain = record.get("subdomain", "@")
            record_id = record.get("record_id", 0)
            fulldomain = domain if subdomain == "@" else subdomain + "." + domain
            dnspod.modify_ddns_record(
                {
                    "Domain": domain,
                    "SubDomain": subdomain,
                    "RecordId": record_id,
                    "RecordLine": "默认",
                }
            )
            logger.info(
                f"Successfully updated {record_ip} to {url_ip} for {fulldomain}."
            )
            db.update_record_value(record.get("record_id", 0), url_ip)
        return True


if __name__ == "__main__":
    ddns(False)
