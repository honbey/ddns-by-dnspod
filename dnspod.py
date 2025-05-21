import sys
import json

from tencentcloud.dnspod.v20210323 import dnspod_client, models
from tencentcloud.common import credential
from tencentcloud.common.exception.tencent_cloud_sdk_exception import (
    TencentCloudSDKException,
)


class DNSPodAPI:
    def __init__(self, key: tuple[str, str]):
        try:
            cred = credential.Credential(key[0], key[1])
            self.client = dnspod_client.DnspodClient(cred, "ap-shanghai")
        except TencentCloudSDKException as e:
            print(e)
        except Exception as e:
            print(e)

    def describe_domain_list(self, data: dict):
        rst = models.DescribeDomainListResponse()
        try:
            req = models.DescribeDomainListRequest()
            req._deserialize(data)
            rst = self.client.DescribeDomainList(req)
        except TencentCloudSDKException as e:
            print(e)
        except Exception as e:
            print(e)
        finally:
            return rst

    def describe_record_list(self, data: dict):
        rst = models.DescribeRecordListResponse()
        try:
            req = models.DescribeRecordListRequest()
            req._deserialize(data)
            rst = self.client.DescribeRecordList(req)
        except TencentCloudSDKException as e:
            print(e)
        except Exception as e:
            print(e)
        finally:
            return rst

    def describe_record_filter_list(self, data: dict):
        rst = models.DescribeRecordFilterListResponse()
        try:
            req = models.DescribeRecordFilterListRequest()
            req._deserialize(data)
            rst = self.client.DescribeRecordFilterList(req)
        except TencentCloudSDKException as e:
            print(e)
        except Exception as e:
            print(e)
        finally:
            return rst

    def describe_record_group_list(self, data: dict):
        rst = models.DescribeRecordGroupListResponse()
        try:
            req = models.DescribeRecordGroupListRequest()
            req._deserialize(data)
            rst = self.client.DescribeRecordGroupList(req)
        except TencentCloudSDKException as e:
            print(e)
        except Exception as e:
            print(e)
        finally:
            return rst

    def create_record(self, data: dict):
        rst = models.CreateRecordResponse()
        try:
            req = models.CreateRecordRequest()
            req._deserialize(data)
            rst = self.client.CreateRecord(req)
        except TencentCloudSDKException as e:
            print(e)
        except Exception as e:
            print(e)
        finally:
            return rst

    def delete_record(self, data: dict):
        rst = models.DeleteRecordResponse()
        try:
            req = models.DeleteRecordRequest()
            req._deserialize(data)
            rst = self.client.DeleteRecord(req)
        except TencentCloudSDKException as e:
            print(e)
        except Exception as e:
            print(e)
        finally:
            return rst

    def describe_record(self, data: dict):
        rst = models.DescribeRecordResponse
        try:
            req = models.DescribeRecordRequest()
            req._deserialize(data)
            rst = self.client.DescribeRecord(req)
        except TencentCloudSDKException as e:
            print(e)
        except Exception as e:
            print(e)
        finally:
            return rst

    def modify_record(self, data: dict):
        rst = models.ModifyRecordResponse()
        try:
            req = models.ModifyRecordRequest()
            req._deserialize(data)
            rst = self.client.ModifyRecord(req)
        except TencentCloudSDKException as e:
            print(e)
        except Exception as e:
            print(e)
        finally:
            return rst

    def modify_ddns_record(self, data: dict):
        rst = models.ModifyDynamicDNSResponse()
        try:
            req = models.ModifyDynamicDNSRequest()
            req._deserialize(data)
            print(data)
            rst = self.client.ModifyDynamicDNS(req)
        except TencentCloudSDKException as e:
            print(e)
        except Exception as e:
            print(e)
        finally:
            return rst
