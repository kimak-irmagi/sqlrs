# Сторонние SQL-датасеты (загружаются по требованию)

## Зависимости движка

### Драйвер PostgreSQL JDBC

Артефакт: `org.postgresql:postgresql:42.7.13` (BSD-2-Clause).
Источник: [Maven Central](https://repo.maven.apache.org/maven2/org/postgresql/postgresql/42.7.13/).
Исходный проект: [pgjdbc](https://github.com/pgjdbc/pgjdbc).
В описании внешних артефактов закреплён SHA-256. Сценарии сборки и выпуска проверяют
загруженный JAR и включают его как `lib/postgresql-42.7.13.jar`; выполнение задания
не загружает замену.

## Примеры с обычным SQL

### Chinook (PostgreSQL)

Источник: lerocha/chinook-database (GitHub)
Файл: Chinook_PostgreSql.sql
URL: <https://raw.githubusercontent.com/lerocha/chinook-database/master/ChinookDatabase/DataSources/Chinook_PostgreSql.sql>

### Sakila (порт для PostgreSQL)

Источник: <https://github.com/sakiladb/postgres> (BSD-2-Clause)
Загружаемые файлы (raw):

- 0-postgres-sakila-setup.sql
- 1-postgres-sakila-schema.sql
- 2-postgres-sakila-insert-data.sql
- 3-postgres-sakila-user.sql

### PostgresPro Demo DB "Airlines / Flights"

Страница источника: <https://postgrespro.com/community/demodb>
Архив: <https://edu.postgrespro.com/demo-small-en.zip>

## Примеры Liquibase

### JHipster Sample App (Liquibase changelog-и)

Источник: <https://github.com/jhipster/jhipster-sample-app> (Apache-2.0)
Загружаемые файлы (raw):

- master.xml
- 00000000000000_initial_schema.xml
- 20150805124838_added_entity_BankAccount.xml
- 20150805124936_added_entity_Label.xml
- 20150805125054_added_entity_Operation.xml
- 20150805124838_added_entity_constraints_BankAccount.xml
- 20150805125054_added_entity_constraints_Operation.xml

### Liquibase GitHub Action Example (formatted SQL changelog)

Источник: <https://github.com/liquibase/liquibase-github-action-example> (Apache-2.0)
Загружаемый файл (raw): samplechangelog.h2.sql
