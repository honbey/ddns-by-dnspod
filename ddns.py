#!/usr/bin/env python3
# _*_ coding:utf-8 _*_
#

import argparse
import logging

import yaml

from db import DomainDatabase
from dnspod import DNSPodAPI
from iptools import IpInfo
from utils import Logger, push2gotify


def ddns(config: str, force_update: bool = False) -> bool:
    with open(config, "r") as f:
        cfg = yaml.safe_load(f)
        database = cfg.get("database")
        ip_config = cfg.get("ip_config")
        gotify = cfg.get("gotify")
    log_level = logging.INFO
    logger = Logger("DDNS by DNSPod", level=log_level)
    db = DomainDatabase(db=database, log_level=log_level)
    record_list = db.query_record_by_group("DDNS")
    one_record = record_list[0]
    record_ip = one_record.get("value", "0.0.0.0")
    domain = one_record.get("domain", "example.com")
    subdomain = one_record.get("subdomain", "@")
    fulldomain = domain if subdomain == "@" else subdomain + "." + domain
    info = IpInfo(config=ip_config)
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
        dnspod = DNSPodAPI(db.key, log_level=log_level)
        msg = ""
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
            msg = f"Successfully updated {record_ip} to {url_ip} for {fulldomain}."
            logger.info(msg)
            db.update_record_value(record.get("record_id", 0), url_ip)
        push2gotify("DDNS by DNSPod", msg, gotify.get("url"), gotify.get("token"))
        return True


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("config", type=str, help="The path of config file")
    args = parser.parse_args()
    ddns(args.config, force_update=True)
