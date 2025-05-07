#!/usr/bin/env python3
# _*_ coding:utf-8 _*_

import os
import argparse
import yaml
import requests

from tencentcloud.common.exception.tencent_cloud_sdk_exception import (
    TencentCloudSDKException,
)

from dnspod import DNSPodAPI

if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "-v", "--version", action="version", version="DDNS by DNSPod API v0.2.0"
    )
    # api args
    parser.add_argument(
        "--domain", type=str, nargs=1, help="specify main domain", dest="domain"
    )
    parser.add_argument(
        "--subdomain",
        type=str,
        nargs="+",
        default="@",
        help="specify subdomain(s)",
        dest="subdomain",
    )
    parser.add_argument("--ip", nargs=1, type=str, help="ip address", dest="ip")
    parser.add_argument(
        "--type", nargs=1, type=str, help="record type", dest="type", default="A"
    )
    parser.add_argument(
        "--line", nargs=1, type=str, help="record line", dest="line", default="默认"
    )
    # log args
    parser.add_argument("--debug", action="store_true", help="debug mode", dest="debug")
    parser.add_argument(
        "--log-file", nargs=1, type=str, help="log file", dest="log_file"
    )
    parser.add_argument(
        "--log-level", nargs=1, type=str, help="log level", dest="log_level"
    )
    parser.add_argument(
        "--log-size", nargs=1, type=str, help="log size", dest="log_size"
    )
    # operation args
    # fmt: off
    parser.add_argument(
        "-c", "--config", type=str, nargs=1, help="specify config file(yaml)", dest="config"
    )
    parser.add_argument(
        "-a", "--add", action="store_true", help="add record", dest="add"
    )
    parser.add_argument(
        "-d", "--ddns", action="store_true", help="enbale DDNS", dest="ddns"
    )
    parser.add_argument(
        "-i", "--info", action="store_true", help="show record infos", dest="info"
    )
    parser.add_argument(
        "-m", "--modify", action="store_true", help="modify record", dest="modify"
    )
    parser.add_argument(
        "-r", "--remove", action="store_true", help="remove/delete record", dest="remove"
    )
    # fmt: on
    args = parser.parse_args()
    try:
        if args.config:
            with open(args.config[0], "r") as f:
                config = yaml.safe_load(f)
                key_id = config["tc_key_id"]
                prikey = config["tc_prikey"]
                args = argparse.Namespace(**config)
                args.add, args.info, args.modify, args.remove = [None, None, None, None]
        else:
            key_id = os.environ.get("TENCENT_API_PUB_KEY")
            prikey = os.environ.get("TENCENT_API_PRI_KEY")

        dnspod = DNSPodAPI(key_id, prikey, args)

        if args.add:
            dnspod.add_record()
        elif args.ddns:
            msg = dnspod.ddns()
            # gotify only enable with a yaml config
            if len(msg) > 3 and args.gotify:
                data = {
                    "title": "DDNS by DNSPod",
                    "message": msg,
                    "priority": args.gotify["priority"],
                }
                requests.post(
                    f"{args.gotify['url']}/message?token={args.gotify['token']}",
                    data=data,
                )
        elif args.info:
            dnspod.info()
        elif args.modify:
            dnspod.mod_record()
        elif args.remove:
            dnspod.del_record()

    except TencentCloudSDKException as e:
        print(e)
