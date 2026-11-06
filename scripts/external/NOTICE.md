# Third-party SQL datasets (fetched on demand)

## Engine dependencies

### PostgreSQL JDBC driver

Artifact: `org.postgresql:postgresql:42.7.13` (BSD-2-Clause).
Source: [Maven Central](https://repo.maven.apache.org/maven2/org/postgresql/postgresql/42.7.13/).
Upstream: [pgjdbc](https://github.com/pgjdbc/pgjdbc).
The external-artifact manifest pins its SHA-256. Build and release scripts verify
the fetched JAR and package it under `lib/postgresql-42.7.13.jar`; job execution
does not download a substitute.

## Plain sql examples

### Chinook (PostgreSQL)

Upstream: lerocha/chinook-database (GitHub)
File: Chinook_PostgreSql.sql
URL: <https://raw.githubusercontent.com/lerocha/chinook-database/master/ChinookDatabase/DataSources/Chinook_PostgreSql.sql>

### Sakila (PostgreSQL port)

Upstream: <https://github.com/sakiladb/postgres> (BSD-2-Clause)
Files fetched (raw):

- 0-postgres-sakila-setup.sql
- 1-postgres-sakila-schema.sql
- 2-postgres-sakila-insert-data.sql
- 3-postgres-sakila-user.sql

### PostgresPro Demo DB “Airlines / Flights”

Upstream page: <https://postgrespro.com/community/demodb>
Archive: <https://edu.postgrespro.com/demo-small-en.zip>

## Liquibase examples

### JHipster Sample App (Liquibase changelogs)

Upstream: <https://github.com/jhipster/jhipster-sample-app> (Apache-2.0)
Files fetched (raw):

- master.xml
- 00000000000000_initial_schema.xml
- 20150805124838_added_entity_BankAccount.xml
- 20150805124936_added_entity_Label.xml
- 20150805125054_added_entity_Operation.xml
- 20150805124838_added_entity_constraints_BankAccount.xml
- 20150805125054_added_entity_constraints_Operation.xml

### Liquibase GitHub Action Example (formatted SQL changelog)

Upstream: <https://github.com/liquibase/liquibase-github-action-example> (Apache-2.0)
File fetched (raw): samplechangelog.h2.sql
