#!/usr/bin/env python3
# _*_ coding:utf-8 _*_
#

import argparse
import os
import time

import tldextract

from dnspod import DNSPodAPI
from utils import Logger

tld_file_path = "file:///opt/data/workspace/ddns-by-dnspod/public_suffix_list.dat"
extractor = tldextract.TLDExtract(cache_dir=None, suffix_list_urls=[tld_file_path])

logger = Logger("Certbot Auth")

if os.getenv("CERTBOT_DOMAIN") is None:
    exit(-1)
else:
    domain = extractor(os.getenv("CERTBOT_DOMAIN"))  # type: ignore
MAIN_DOMAIN = f"{domain.domain}.{domain.suffix}"
SUBDOMAIN = domain.subdomain
if SUBDOMAIN == "":
    TXTHOST = "_acme-challenge"
else:
    TXTHOST = f"_acme-challenge.{SUBDOMAIN}"
CERTBOT_VALIDATION = os.getenv("CERTBOT_VALIDATION")

RECORD_PATH = f"/tmp/CERTBOT_{MAIN_DOMAIN}"
RECORD_FILE = f"{RECORD_PATH}/RECORD_ID_{CERTBOT_VALIDATION}"

data = {
    "Domain": MAIN_DOMAIN,
    "SubDomain": TXTHOST,
    "Value": CERTBOT_VALIDATION,
    "RecordLine": "默认",
    # "RecordType": "TXT",
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
        (os.getenv("TENCENTCLOUD_API_PUB_KEY"), os.getenv("TENCENTCLOUD_API_PRI_KEY"))  # type: ignore
    )

    logger.info(data)

    if args.clean:
        with open(RECORD_FILE) as f:
            record_id = int(f.readline())
        dnspod.delete_record({"Domain": MAIN_DOMAIN, "RecordId": record_id})
        os.remove(RECORD_FILE)
        logger.info("_acme-challenge record has been _deleted_.")
    else:
        resp = dnspod.create_txt_record(data)
        record_id = resp.RecordId
        if not os.path.exists(RECORD_PATH):
            os.makedirs(RECORD_PATH, 0o700)
        with open(RECORD_FILE, "w") as f:
            f.write(str(record_id))
        logger.info("_acme-challenge record has been *created*.")

        time.sleep(20)
