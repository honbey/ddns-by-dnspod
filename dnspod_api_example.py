import os

# import json
import yaml

from tencentcloud.dnspod.v20210323 import dnspod_client, models

from tencentcloud.common import credential
from tencentcloud.common.exception.tencent_cloud_sdk_exception import (
    TencentCloudSDKException,
)

try:
    with open("example.yaml", "r") as f:
        config = yaml.safe_load(f)
        domain = config["domain"]
    cred = credential.Credential(
        os.environ.get("TENCENT_API_PUB_KEY"), os.environ.get("TENCENT_API_PRI_KEY")
    )

    client = dnspod_client.DnspodClient(cred, "ap-shanghai")

    req = models.DescribeDomainRequest()
    req.Domain = domain

    resp = client.DescribeDomain(req)
    print(resp.to_json_string())

except TencentCloudSDKException as e:
    print(e)
