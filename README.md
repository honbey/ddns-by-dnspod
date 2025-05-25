# DDNS by DNSPod

Using [Tencent Cloud SDK for python](https://docs.dnspod.cn/api/api3/) to DDNS.

## Manual auth hook of Certbot

`certbot_auth.py` refactored code from [al-one/certbot-auth-dnspod](https://github.com/al-one/certbot-auth-dnspod) by Python.

Improvements:

- Support multi-level top domain such as com.cn

### Usage

```bash
certbot certonly -d "test.example.com" \
  --manual --preferred-challenges dns-01 \
  --server https://acme-v02.api.letsencrypt.org/directory \
  --manual-auth-hook /opt/data/workspace/ddns-by-dnspod/certbot_auth.py \
  --manual-cleanup-hook "/opt/data/workspace/ddns-by-dnspod/certbot_auth.py --clean" \
  --config-dir /opt/data/etc/letsencrypt \
  --work-dir /opt/data/etc/letsencrypt \
  --logs-dir /opt/data/log/letsencrypt --dry-run
```
