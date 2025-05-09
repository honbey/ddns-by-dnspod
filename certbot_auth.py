#!/usr/bin/env python3
# _*_ coding:utf-8 _*_
#

import os
import time
import argparse
import tldextract

from dnspod import DNSPodAPI

tld_file_path = "file:///opt/data/workspace/ddns-by-dnspod/public_suffix_list.dat"
extractor = tldextract.TLDExtract(cache_dir=None, suffix_list_urls=[tld_file_path])

domain = extractor(os.getenv("CERTBOT_DOMAIN"))
MAIN_DOMAIN = f"{domain.domain}.{domain.suffix}"
SUBDOMAIN = domain.subdomain
if SUBDOMAIN == "":
    TXTHOST = "_acme-challenge"
else:
    TXTHOST = f"_acme-challenge.{SUBDOMAIN}"
CERTBOT_VALIDATION = os.getenv("CERTBOT_VALIDATION")

RECORD_PATH = f"/tmp/CERTBOT_{MAIN_DOMAIN}"
RECORD_FILE = f"{RECORD_PATH}/RECORD_ID_{CERTBOT_VALIDATION}"

config = {
    "domain": MAIN_DOMAIN,
    "subdomain": TXTHOST,
    "value": CERTBOT_VALIDATION,
    "type": "TXT",
    "line": "默认",
}

if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--clean",
        action="store_true",
        default=False,
        help="Clean authorized record",
        dest="clean",
    )
    args = parser.parse_args()

    dnspod = DNSPodAPI(
        os.getenv("TENCENT_API_PUB_KEY"),
        os.getenv("TENCENT_API_PRI_KEY"),
        argparse.Namespace(**config),
    )

    print(config)

    if args.clean:
        with open(RECORD_FILE) as f:
            record_id = int(f.readline())
        dnspod.del_record(record_id=record_id)
        os.remove(RECORD_FILE)
        print("_acme-challenge record has been _deleted_")
    else:
        resp = dnspod.add_record()
        if resp:
            record_id = resp.get("Response", {}).get("RecordId", -1)
        else:
            record_id = -1
        if not os.path.exists(RECORD_PATH):
            os.makedirs(RECORD_PATH, 0o700)
        with open(RECORD_FILE, "w") as f:
            f.write(str(record_id))
        print("_acme-challenge record has been *created*")

        time.sleep(20)
