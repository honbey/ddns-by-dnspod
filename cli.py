#!/usr/bin/env python3
# _*_ coding:utf-8 _*_

import os
import argparse

from dnspod import DNSPodAPI

if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    # fmt: off
    parser.add_argument(
        "-v", "--version", action="version", version="DDNS by DNSPod API v0.2.1"
    )
    # api args
    parser.add_argument(
        "--domain", type=str, nargs=1, help="specify main domain", dest="domain"
    )
    parser.add_argument(
        "--subdomain", type=str, nargs=1, help="specify subdomain", dest="subdomain",
    )
    parser.add_argument(
        "--value", "--ip", nargs=1, type=str, help="record value", dest="value"
    )
    parser.add_argument(
        "--type", nargs=1, type=str, help="record type", dest="type", default="A"
    )
    parser.add_argument(
        "--line", nargs=1, type=str, help="record line", dest="line", default="默认"
    )
    # operation args
    parser.add_argument(
        "-c", "--create", action="store_true", help="create record", dest="create"
    )
    parser.add_argument(
        "-d", "--delete", action="store_true", help="delete record", dest="delete"
    )
    parser.add_argument(
        "-D", "--ddns", action="store_true", help="enbale DDNS", dest="ddns"
    )
    parser.add_argument(
        "-i", "--info", action="store_true", help="show record infos", dest="info"
    )
    parser.add_argument(
        "-m", "--modify", action="store_true", help="modify record", dest="modify"
    )
    # fmt: on
    args = parser.parse_args()

    key_id = os.environ.get("TENCENT_API_PUB_KEY")
    prikey = os.environ.get("TENCENT_API_PRI_KEY")

    dnspod = DNSPodAPI((key_id, prikey)) # type: ignore

    if args.create:
        dnspod.create_record({
            "Domain": args.domain[0],
            "SubDomain": args.subdomain[0],
            "RecordType": args.type[0],
            "RecordLine": args.line[0],
            "Value": args.value[0]
        })
    else:
        pass
