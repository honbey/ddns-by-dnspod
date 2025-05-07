import os
import argparse

from tencentcloud.common.exception.tencent_cloud_sdk_exception import (
    TencentCloudSDKException,
)

from .dnspod import DNSPodAPI

if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "-v", "--version", action="version", version="DDNS by DNSPod API v0.1.0"
    )
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
    # fmt: off
    # TODO: get config from file and operate
    parser.add_argument(
        "-c", "--config", type=str, nargs=1, help="specify config file(yaml)", dest="config"
    )
    parser.add_argument(
        "-a", "--add", action="store_true", help="add record", dest="add"
    )
    parser.add_argument(
        "-d", "--ddns", action="store_true", help="enbale DDNS", dest="enable"
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
        dnspod = DNSPodAPI(
            os.environ.get("TENCENT_API_PUB_KEY"),
            os.environ.get("TENCENT_API_PRI_KEY"),
            args,
        )

        if args.add:
            dnspod.add_record()
        elif args.ddns:
            dnspod.ddns()
        elif args.info:
            dnspod.info()
        elif args.modify:
            dnspod.mod_record()
        elif args.remove:
            dnspod.del_record()

    except TencentCloudSDKException as e:
        print(e)
