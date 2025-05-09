# DDNS by DNSPod

Using [Tencent Cloud SDK for python](https://docs.dnspod.cn/api/api3/) to DDNS.

## Common SDK

For the purpose of script lightweighting, I chose to call the API using the common SDK.

### A Example not using common SDK

```python
import os
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
```

## Manual auth hook of Certbot

`certbot_auth.py` refactored code from [al-one/certbot-auth-dnspod](https://github.com/al-one/certbot-auth-dnspod) by Python.

Improvements:

- Support multi-level top domain such as com.cn
