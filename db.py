import glob
import logging
import os
import sqlite3

from datetime import datetime
from typing import ParamSpecKwargs

from dnspod import DNSPodAPI
from utils import Logger


class DomainDatabase:
    def __init__(
        self,
        db: str = "",
        key: tuple[str, str] = ("", ""),
        log_level: int = logging.INFO,
    ):
        self.logger = Logger("DomainDatabase", level=log_level)
        self._this_dir = os.path.dirname(os.path.abspath(__file__))
        if db == "":
            self._db = sqlite3.connect(os.path.join(self._this_dir, "data.db"))
        else:
            self._db = sqlite3.connect(db)
        self._db.row_factory = sqlite3.Row
        if any(s == "" for s in key):
            cur = self._db.cursor()
            cur.execute("SELECT * FROM dnspod_auth WHERE enable = ?;", (True,))
            auth = dict(cur.fetchone())
            self._key = (auth.get("pub_key", ""), auth.get("pri_key", ""))
        else:
            self._key = key
            # TODO: insert key into table if key_id is not exists
        self.dnspod = DNSPodAPI(self._key)

    @property
    def db(self):
        return self._db

    @property
    def key(self):
        return self._key

    def create_tbl(self):
        self.logger.debug("Creating tables...")
        sql_dir = os.path.join(self._this_dir, "sql")
        sql_files = glob.glob(os.path.join(sql_dir, "table_*.sql"))
        cur = self._db.cursor()
        for i in sql_files:
            self.logger.debug(f"Creating table - [{i}] ...")
            with open(i) as f:
                cur.execute(f.read())

        self._db.commit()
        self.logger.debug("Tables have been created.")
        cur.close()

    def init_tbl(self):
        self.logger.debug("Initializing tables...")
        cur = self._db.cursor()
        # dnspod_auth
        _ = self._init_tbl_auth(cur, self.key)
        # dnspod_domain
        _, domain_id = self._init_tbl_domain(cur)
        # dnspod_record
        _ = self._init_tbl_record(cur, domain_id)
        # dnspod_record_group
        _ = self._init_tbl_record_group(cur, domain_id)
        self._db.commit()
        self.logger.debug("Tables have been initialized.")
        cur.close()

    def _init_tbl_auth(self, cur: sqlite3.Cursor, key: tuple[str, str]):
        self.logger.debug("Initializing table - [dnspod_auth] ...")
        cur.execute(
            """
            INSERT INTO dnspod_auth(pub_key, pri_key)
            VALUES(?, ?);
            """,
            key,
        )
        return True

    def _init_tbl_domain(self, cur: sqlite3.Cursor):
        self.logger.debug("Initializing table - [dnspod_domain] ...")
        resp = self.dnspod.describe_domain_list({})
        domain_list = resp.DomainList
        domain_id = []
        if isinstance(domain_list, list):
            for domain in domain_list:
                cur.execute(
                    """
                    INSERT INTO dnspod_domain(
                        domain_id, domain, created_on,
                        updated_on, grade
                    )
                    VALUES(?, ?, ?, ?, ?);
                    """,
                    (
                        domain.DomainId,
                        domain.Name,
                        domain.CreatedOn,
                        domain.UpdatedOn,
                        domain.Grade,
                    ),
                )
                domain_id.append(domain.DomainId)
        else:
            return False, domain_id
        return True, domain_id

    def _init_tbl_record(self, cur: sqlite3.Cursor, domain_id: list):
        self.logger.debug("Initializing table - [dnspod_record] ...")
        for d_id in domain_id:
            resp = self.dnspod.describe_record_list(
                {
                    "Domain": "",  # the priority of DomainId is higher than Domain
                    "DomainId": d_id,
                }
            )
            record_list = resp.RecordList
            if isinstance(record_list, list) and len(record_list) > 0:
                for record in record_list:
                    cur.execute(
                        """
                        INSERT INTO dnspod_record(
                            domain_id, record_id , subdomain,
                            type, line, line_id,
                            value, mx, ttl,
                            weight, status, remark,
                            default_ns, updated_on, monitor_status
                        )
                        VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
                        """,
                        (
                            d_id,
                            record.RecordId,
                            record.Name,
                            record.Type,
                            record.Line,
                            record.LineId,
                            record.Value,
                            record.MX,
                            record.TTL,
                            record.Weight,
                            record.Status,
                            record.Remark,
                            record.DefaultNS,
                            record.UpdatedOn,
                            record.MonitorStatus,
                        ),
                    )
        return True

    def _init_tbl_record_group(self, cur: sqlite3.Cursor, domain_id: list):
        self.logger.debug("Initializing table - [dnspod_record_group] ...")
        for d_id in domain_id:
            resp = self.dnspod.describe_record_group_list(
                {
                    "Domain": "",  # the priority of DomainId is higher than Domain
                    "DomainId": d_id,
                }
            )
            group_list = resp.GroupList
            if isinstance(group_list, list) and len(group_list) > 0:
                for group in group_list:
                    cur.execute(
                        """
                        INSERT INTO dnspod_record_group(
                            domain_id, group_id, name, type
                        )
                        VALUES(?, ?, ?, ?);
                        """,
                        (d_id, group.GroupId, group.GroupName, group.GroupType),
                    )
        return True

    def query_record_by_group(self, group_name="默认分组"):
        cur = self._db.cursor()
        cur.execute(
            """
            SELECT group_id FROM dnspod_record_group
            WHERE name = ?;
            """,
            (group_name,),
        )
        row = dict(cur.fetchone())
        cur.execute(
            """
            SELECT a.*, b.domain FROM dnspod_record AS a, dnspod_domain AS b
            WHERE group_id = ? AND a.domain_id = b.domain_id;
            """,
            (row.get("group_id"),),
        )
        return [dict(ctx) for ctx in cur.fetchall()]

    def update_record_group(self):
        cur = self._db.cursor()
        cur.execute("SELECT * FROM dnspod_record_group;")
        for i in cur.fetchall():
            d = dict(i)
            if d.get("group_id") != 0:
                resp = self.dnspod.describe_record_filter_list(
                    {
                        "Domain": "",  # the priority of DomainId is higher than Domain
                        "DomainId": d.get("domain_id"),
                        "GroupId": [d.get("group_id")],
                    }
                )
                record_list = resp.RecordList
                if isinstance(record_list, list) and len(record_list) > 0:
                    for record in record_list:
                        cur.execute(
                            """
                            UPDATE dnspod_record
                            SET
                                group_id = ?, comment = ?
                            WHERE
                                domain_id = ? AND record_id = ?;
                            """,
                            (
                                d.get("group_id"),
                                d.get("name"),
                                d.get("domain_id"),
                                record.RecordId,
                            ),
                        )
        self._db.commit()
        cur.close()

    def update_dnspod_record(self, record_id: int, value: str):
        cur = self._db.cursor()
        cur.execute(
            """
            UPDATE dnspod_record
            SET
                value = ?,
                updated_on = datetime('now', 'localtime')
            WHERE
                record_id = ?;
            """,
            (value, record_id),
        )
        self._db.commit()
        cur.close()

    def insert_ddns_record(self, ipv4: str, ipv6: str = "::1"):
        cur = self._db.cursor()
        cur.execute("SELECT id, updated_on FROM ddns_record ORDER BY id DESC;")
        record = cur.fetchone()
        curr_time = datetime.now()
        if record is None:
            duration = 1
        else:
            _, prev_time = record
            prev_time = datetime.strptime(prev_time, "%Y-%m-%d %H:%M:%S")
            duration = int((curr_time - prev_time).total_seconds())
        cur.execute(
            """
            INSERT INTO ddns_record(
                updated_on, ipv4_addr, ipv6_addr, duration
            )
            VALUES(?, ?, ?, ?);
            """,
            (curr_time.strftime("%Y-%m-%d %H:%M:%S"), ipv4, ipv6, duration),
        )
        self._db.commit()
        cur.close()


if __name__ == "__main__":
    key = (
        os.environ.get("TENCENTCLOUD_API_PUB_KEY"),
        os.environ.get("TENCENTCLOUD_API_PRI_KEY"),
    )
    if all(isinstance(k, str) for k in key):
        d = DomainDatabase()  # type: ignore
        d.create_tbl()
        # d.init_tbl()
        # d.update_record_group()
        # print(d.query_record_by_group("DDNS"))
