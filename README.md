# DDNS by DNSPod

Using [Tencent Cloud SDK for python](https://docs.dnspod.cn/api/api3/) to DDNS.

## Manual auth hook of Certbot

`certbot_auth.py` refactored code from [al-one/certbot-auth-dnspod](https://github.com/al-one/certbot-auth-dnspod) by Python.

Improvements:

- Support multi-level top domain such as com.cn
