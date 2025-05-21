CREATE TABLE dnspod_record_group (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    domain_id INTEGER NOT NULL,
    group_id INTEGER NOT NULL,
    name VARCHAR(100) NOT NULL,
    type VARCHAR(8) NOT NULL,
    enable BOOLEAN DEFAULT TRUE,
    comment TEXT NULL,
    FOREIGN KEY(domain_id) REFERENCES dnspod_domain(domain_id)
);
