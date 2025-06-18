CREATE TABLE IF NOT EXISTS dnspod_domain(
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    domain_id INTEGER NOT NULL,
    domain VARCHAR(100) NOT NULL UNIQUE,
    created_on DATETIME,
    updated_on DATETIME,
    grade VARCHAR(20),
    enable BOOLEAN DEFAULT TRUE,
    comment TEXT NULL
);
