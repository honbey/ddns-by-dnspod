import os

from iptools import IpInfo
from db import DomainDatabase
from dnspod import DNSPodAPI


def ddns() -> str:
    key = (
        os.environ.get("TENCENTCLOUD_API_PUB_KEY"),
        os.environ.get("TENCENTCLOUD_API_PRI_KEY"),
    )
    db = DomainDatabase()
    record_list = db.query_record_by_group("DDNS")
    one_record = record_list[0]
    record_ip = one_record.get("value", "0.0.0.0")
    domain = one_record.get("domain", "example.com")
    subdomain = one_record.get("subdomain", "@")
    fulldomain = domain if subdomain == "@" else subdomain + "." + domain
    info = IpInfo()
    url_ip = str(info.get_ip())
    dns_ip = str(info.dns_resolve(fulldomain))
    print(f"URL IP: [{url_ip}], DNS IP: [{dns_ip}], record IP: [{record_ip}]")
    if IpInfo.judge_ip(url_ip) and url_ip == dns_ip:
        print("URL IP is equal to DNS IP, skipping ddns")
        return "Skip."
    elif IpInfo.judge_ip(url_ip) and url_ip == record_ip:
        print("URL IP is equal to record IP, skipping ddns")
        return "Skip."
    else:
        dnspod = DNSPodAPI(key)  # type: ignore
        for record in record_list:
            dnspod.modify_ddns_record(
                {
                    "Domain": domain,
                    "SubDomain": record.get("subdomain"),
                    "RecordId": record.get("record_id"),
                    "RecordLine": "默认",
                }
            )
            db.update_record_value(record.get("record_id", 0), url_ip)
        return f"Successfully updated {record_ip} to {url_ip}."


if __name__ == "__main__":
    ddns()
